package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/semsemyonoff/cueBreaker-backend/internal/cue"
	"github.com/semsemyonoff/cueBreaker-backend/internal/job"
	"github.com/semsemyonoff/cueBreaker-backend/internal/joblog"
	"github.com/semsemyonoff/cueBreaker-backend/internal/scan"
	"github.com/semsemyonoff/cueBreaker-backend/internal/split"
)

// pathRequest is the shared JSON body of the preview and split endpoints: a
// scan-relative directory path plus the CUE file name within it.
type pathRequest struct {
	Path    string `json:"path"`
	CueFile string `json:"cue_file"`
}

// previewResponse is the extended preview payload: cue.Album's fields plus
// cover/split-status/duration data the waveform needs.
type previewResponse struct {
	cue.Album
	HasCover     bool    `json:"has_cover"`
	CoverName    string  `json:"cover_name,omitempty"`
	SplitDone    bool    `json:"split_done"`
	OutputTracks int     `json:"output_tracks"`
	TotalSeconds float64 `json:"total_seconds"`
}

// statusResponse is the wire shape of a job's current state.
type statusResponse struct {
	Status          job.Status     `json:"status"`
	Message         string         `json:"message"`
	ResultFiles     []string       `json:"result_files"`
	ProgressCurrent int            `json:"progress_current"`
	ProgressTotal   int            `json:"progress_total"`
	ProgressDetail  string         `json:"progress_detail"`
	Log             []joblog.Entry `json:"log"`
	LogNext         int            `json:"log_next"`
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	result, err := scan.FindPairs(s.cfg.InputDir, s.cfg.OutputDir)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.logger.Info("scan complete", "pairs", len(result.Pairs))
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusOK, []scan.Pair{})
		return
	}

	result, err := scan.FindPairs(s.cfg.InputDir, s.cfg.OutputDir)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, scan.Search(result.Pairs, q))
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodePathRequest(w, r)
	if !ok {
		return
	}

	relPath, ok := s.scanRelPath(w, req.Path)
	if !ok {
		return
	}

	absDir := filepath.Join(s.cfg.InputDir, relPath)
	cuePath := filepath.Join(absDir, req.CueFile)

	if !s.cueFileContained(w, cuePath) {
		return
	}

	album, err := cue.Parse(cuePath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if album.Tracks == nil {
		album.Tracks = []cue.Track{}
	}

	var totalSeconds float64
	if sourcePath, ok := cue.SourceFLAC(album, absDir); ok {
		totalSeconds, _ = cue.TotalSeconds(sourcePath)
	}

	coverName := ""
	coverPath, hasCover := scan.FindCover(absDir)
	if hasCover {
		coverName = filepath.Base(coverPath)
	}

	done, outputTracks := scan.CheckOutputStatus(s.cfg.OutputDir, relPath, cuePath)

	writeJSON(w, http.StatusOK, previewResponse{
		Album:        album,
		HasCover:     hasCover,
		CoverName:    coverName,
		SplitDone:    done,
		OutputTracks: outputTracks,
		TotalSeconds: totalSeconds,
	})
}

func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	dirPath := r.PathValue("path")
	absDir := filepath.Join(s.cfg.InputDir, dirPath)

	if _, ok := s.containedRealPath(absDir); !ok {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	coverPath, ok := scan.FindCover(absDir)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, coverPath)
}

func (s *Server) handleSplit(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodePathRequest(w, r)
	if !ok {
		return
	}

	relPath, ok := s.scanRelPath(w, req.Path)
	if !ok {
		return
	}

	absDir := filepath.Join(s.cfg.InputDir, relPath)
	cuePath := filepath.Join(absDir, req.CueFile)

	if !s.cueFileContained(w, cuePath) {
		return
	}

	jobID := job.JobID(relPath, req.CueFile)
	opts := split.Options{
		CuePath:   cuePath,
		SourceDir: absDir,
		OutDir:    filepath.Join(s.cfg.OutputDir, relPath),
	}

	switch err := s.jobs.Enqueue(jobID, opts); {
	case err == nil:
	case errors.Is(err, job.ErrDuplicate):
		s.logger.Warn("split already in progress", "job_id", jobID)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Already in progress", "job_id": jobID})
		return
	default:
		// Queue full or shutting down: the album has no job of its own, so
		// there is no job_id to poll — only a later retry can help.
		s.logger.Warn("split refused", "job_id", jobID, "reason", err)
		s.writeError(w, http.StatusServiceUnavailable, "The splitter cannot accept new jobs right now. Try again shortly.")
		return
	}

	s.logger.Info("split enqueued", "job_id", jobID, "cue", cuePath)
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "queued"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("job_id")

	state, ok := s.jobs.Get(jobID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	resultFiles := state.ResultFiles
	if resultFiles == nil {
		resultFiles = []string{}
	}

	logSince := parseLogSince(r.URL.Query().Get("log_since"))
	entries, logNext := state.Log.Since(logSince)
	if entries == nil {
		entries = []joblog.Entry{}
	}

	writeJSON(w, http.StatusOK, statusResponse{
		Status:          state.Status,
		Message:         state.Message,
		ResultFiles:     resultFiles,
		ProgressCurrent: state.ProgressCurrent,
		ProgressTotal:   state.ProgressTotal,
		ProgressDetail:  state.ProgressDetail,
		Log:             entries,
		LogNext:         logNext,
	})
}

// parseLogSince parses the log_since query parameter into a cursor, treating
// a missing, negative, or unparseable value as 0 so the first request after
// a page reload is self-sufficient and returns the whole retained buffer.
func parseLogSince(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.info)
}

// decodePathRequest decodes a pathRequest JSON body, writing a 400 response
// and reporting ok=false on failure.
func (s *Server) decodePathRequest(w http.ResponseWriter, r *http.Request) (pathRequest, bool) {
	var req pathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return pathRequest{}, false
	}
	return req, true
}

// scanRelPath validates p as a safe scan-relative directory path and returns
// its cleaned form, writing a 403 and reporting ok=false otherwise. The same
// path is joined onto BOTH the input and output roots, so cueFileContained's
// realpath check on the input side is not enough on its own: an unsanitized
// ".." that still resolves under INPUT_DIR (e.g. "../input/Album") would
// escape OUTPUT_DIR when joined there, redirecting split output outside the
// output root. Rejecting absolute paths and any ".." that escapes the root
// keeps the two joins in the same relative subtree.
func (s *Server) scanRelPath(w http.ResponseWriter, p string) (string, bool) {
	if !filepath.IsAbs(p) {
		clean := filepath.Clean(p)
		if clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return clean, true
		}
	}
	s.writeError(w, http.StatusForbidden, "Invalid path")
	return "", false
}

// cueFileContained checks that cuePath names an existing file whose real
// path stays under the input directory, writing the appropriate 404/403
// error response and returning false if not.
func (s *Server) cueFileContained(w http.ResponseWriter, cuePath string) bool {
	info, err := os.Stat(cuePath)
	if err != nil || info.IsDir() {
		s.writeError(w, http.StatusNotFound, "CUE file not found")
		return false
	}

	if _, ok := s.containedRealPath(cuePath); !ok {
		s.writeError(w, http.StatusForbidden, "Invalid path")
		return false
	}

	return true
}

// containedRealPath resolves target's real path (symlinks included) and
// reports whether it stays under the server's real input directory: either
// equal to it, or nested under it with a path-separator boundary. This is a
// deliberate tightening vs the original trailing-slash-less prefix check —
// it rejects sibling directories that merely share a string prefix (e.g.
// "/input-evil" vs "/input").
func (s *Server) containedRealPath(target string) (string, bool) {
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", false
	}
	if real == s.realInputDir || strings.HasPrefix(real, s.realInputDir+string(filepath.Separator)) {
		return real, true
	}
	return "", false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
