// Package server exposes cueBreaker's HTTP API and serves the embedded SPA.
package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"git.horn/cueBreaker/backend/internal/config"
	"git.horn/cueBreaker/backend/internal/job"
)

// Server implements http.Handler for cueBreaker's JSON API.
type Server struct {
	cfg     config.Config
	jobs    *job.Manager
	version string
	logger  *slog.Logger

	// realInputDir is cfg.InputDir with symlinks resolved, computed once at
	// construction; every path-security check below is relative to it.
	realInputDir string

	static http.Handler
	mux    *http.ServeMux
}

// New builds a Server for cfg, backed by jobs and reporting version at
// GET /api/version. cfg.InputDir must exist and be resolvable.
func New(cfg config.Config, jobs *job.Manager, version string, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	realInputDir, err := filepath.EvalSymlinks(cfg.InputDir)
	if err != nil {
		return nil, fmt.Errorf("server: resolve input dir %q: %w", cfg.InputDir, err)
	}

	static, err := newStaticHandler()
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:          cfg,
		jobs:         jobs,
		version:      version,
		logger:       logger,
		realInputDir: realInputDir,
		static:       static,
		mux:          http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/scan", s.handleScan)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("POST /api/preview", s.handlePreview)
	s.mux.HandleFunc("GET /api/cover/{path...}", s.handleCover)
	s.mux.HandleFunc("POST /api/split", s.handleSplit)
	s.mux.HandleFunc("GET /api/status/{job_id...}", s.handleStatus)
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.Handle("/", s.static)
}

// ServeHTTP dispatches to the API mux, logging every request's method, path,
// status and duration.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	s.mux.ServeHTTP(rec, r)
	s.logger.Info("request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", rec.status,
		"duration", time.Since(start),
	)
}

// statusRecorder wraps an http.ResponseWriter to capture the status code
// written, for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
