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
	"git.horn/cueBreaker/backend/internal/server/openapi"
)

// BuildInfo is the version payload served at GET /api/version: cueBreaker's
// own version plus the versions of the external tools it splits with.
type BuildInfo struct {
	// App is the link-time application version ("dev" in a dev build).
	App string `json:"version"`
	// Shntool is the installed shntool version, omitted when it could not
	// be determined (see split.ShntoolVersion).
	Shntool string `json:"shntool_version,omitempty"`
}

// Server implements http.Handler for cueBreaker's JSON API.
type Server struct {
	cfg    config.Config
	jobs   *job.Manager
	info   BuildInfo
	logger *slog.Logger

	// realInputDir is cfg.InputDir with symlinks resolved, computed once at
	// construction; every path-security check below is relative to it.
	realInputDir string

	static http.Handler
	mux    *http.ServeMux
}

// New builds a Server for cfg, backed by jobs and reporting info at
// GET /api/version. cfg.InputDir must exist and be resolvable.
func New(cfg config.Config, jobs *job.Manager, info BuildInfo, logger *slog.Logger) (*Server, error) {
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
		info:         info,
		logger:       logger,
		realInputDir: realInputDir,
		static:       static,
		mux:          http.NewServeMux(),
	}
	s.routes()
	return s, nil
}

// apiRoute pairs an http.ServeMux pattern with the handler serving it.
type apiRoute struct {
	pattern string
	handler http.HandlerFunc
}

// apiRoutes is the API surface: routes() registers exactly this list, and
// openapi_test.go checks it against internal/server/openapi's hand-written
// spec in both directions. Adding a route here without documenting it (or
// vice versa) fails that test.
func (s *Server) apiRoutes() []apiRoute {
	return []apiRoute{
		{"GET /api/scan", s.handleScan},
		{"GET /api/search", s.handleSearch},
		{"POST /api/preview", s.handlePreview},
		{"GET /api/cover/{path...}", s.handleCover},
		{"POST /api/split", s.handleSplit},
		{"GET /api/status/{job_id...}", s.handleStatus},
		{"GET /api/version", s.handleVersion},
		{"GET " + openapi.SpecURL, s.handleOpenAPISpec},
		{"GET /api/docs", s.handleDocs},
		{"GET " + openapi.BundleURL, s.handleScalarBundle},
	}
}

func (s *Server) routes() {
	for _, route := range s.apiRoutes() {
		s.mux.HandleFunc(route.pattern, route.handler)
	}
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
