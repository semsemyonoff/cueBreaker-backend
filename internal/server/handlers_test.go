package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.horn/cueBreaker/backend/internal/config"
	"git.horn/cueBreaker/backend/internal/job"
	"git.horn/cueBreaker/backend/internal/joblog"
	"git.horn/cueBreaker/backend/internal/scan"
	"git.horn/cueBreaker/backend/internal/split"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// buildWavHeader constructs a minimal valid RIFF/WAVE header (fmt + data
// chunks, no sample bytes), letting tests exercise TotalSeconds without a
// real audio file or the metaflac binary.
func buildWavHeader(sampleRate, byteRate, dataSize uint32) []byte {
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)
	return header
}

const twoTrackCue = `PERFORMER "Album Artist"
TITLE "Album Title"
FILE "album.wav" WAVE
  TRACK 01 AUDIO
    TITLE "First"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "Second"
    INDEX 01 02:00:00
`

// testServer builds a Server rooted at fresh temp input/output dirs,
// returning it alongside those dirs for fixture setup. splitFn defaults to
// a no-op success when nil, so /api/split tests never shell out to real
// tools.
func testServer(t *testing.T, splitFn job.SplitFunc) (*Server, string, string) {
	t.Helper()
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	if splitFn == nil {
		splitFn = func(ctx context.Context, opts split.Options) ([]string, error) {
			return []string{"01 - First.flac", "02 - Second.flac"}, nil
		}
	}

	mgr := job.NewManager(context.Background(), splitFn)
	cfg := config.Config{InputDir: inputDir, OutputDir: outputDir, Port: 5000}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	s, err := New(cfg, mgr, BuildInfo{App: "test-version", Shntool: "9.9.9"}, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, inputDir, outputDir
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
		t.Fatalf("decode response %q: %v", rr.Body.String(), err)
	}
}

func TestHandleScan(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))
	// A rejected directory so the response's log/summary fields are non-trivial.
	writeFile(t, filepath.Join(inputDir, "Rejected", "album.cue"), twoTrackCue)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/scan", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body)
	}
	var result scan.Result
	decodeJSON(t, rr, &result)
	if len(result.Pairs) != 1 || result.Pairs[0].Path != "Album" {
		t.Fatalf("items = %+v, want one entry for Album", result.Pairs)
	}
	if len(result.Log) == 0 {
		t.Fatalf("log = %+v, want at least the scan bookend entries", result.Log)
	}
	if result.Summary.Albums != 1 || result.Summary.Skipped != 1 {
		t.Fatalf("summary = %+v, want albums=1 skipped=1", result.Summary)
	}
}

func TestHandleSearch_EmptyQuery(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search", nil))

	var pairs []scan.Pair
	decodeJSON(t, rr, &pairs)
	if len(pairs) != 0 {
		t.Fatalf("pairs = %+v, want none for empty q", pairs)
	}
}

func TestHandleScan_FindPairsError(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	// FindPairs only fails on an unreadable input root; the server resolved
	// it at construction, so removing it now reaches the 500 branch.
	if err := os.RemoveAll(inputDir); err != nil {
		t.Fatalf("RemoveAll(%q): %v", inputDir, err)
	}

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/scan", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rr.Code, rr.Body)
	}
}

func TestHandleSearch_FindPairsError(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	if err := os.RemoveAll(inputDir); err != nil {
		t.Fatalf("RemoveAll(%q): %v", inputDir, err)
	}

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?q=album", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rr.Code, rr.Body)
	}
}

func TestHandleSearch_Match(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?q=album", nil))

	var pairs []scan.Pair
	decodeJSON(t, rr, &pairs)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v, want one match", pairs)
	}
}

func TestHandleVersion(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	var body map[string]string
	decodeJSON(t, rr, &body)
	if body["version"] != "test-version" {
		t.Fatalf("version = %q, want test-version", body["version"])
	}
	if body["shntool_version"] != "9.9.9" {
		t.Fatalf("shntool_version = %q, want 9.9.9", body["shntool_version"])
	}
}

func TestHandleVersion_OmitsUnknownShntool(t *testing.T) {
	s, _, _ := testServer(t, nil)
	s.info.Shntool = ""

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	var body map[string]string
	decodeJSON(t, rr, &body)
	if _, ok := body["shntool_version"]; ok {
		t.Fatalf("body = %+v, want no shntool_version key", body)
	}
}

func TestHandlePreview_TotalSecondsFromResolvedSource(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 176400))) // 2s

	body, _ := json.Marshal(pathRequest{Path: "Album", CueFile: "album.cue"})
	req := httptest.NewRequest(http.MethodPost, "/api/preview", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body)
	}

	var resp previewResponse
	decodeJSON(t, rr, &resp)
	if resp.TotalSeconds != 2.0 {
		t.Fatalf("total_seconds = %v, want 2.0", resp.TotalSeconds)
	}
	if len(resp.Tracks) != 2 {
		t.Fatalf("tracks = %+v, want 2", resp.Tracks)
	}
	if resp.Tracks[1].StartSeconds != 120 {
		t.Fatalf("track 2 start_seconds = %v, want 120", resp.Tracks[1].StartSeconds)
	}
	if resp.Performer != "Album Artist" {
		t.Fatalf("performer = %q, want Album Artist", resp.Performer)
	}
}

func TestHandlePreview_MissingCue(t *testing.T) {
	s, _, _ := testServer(t, nil)

	body, _ := json.Marshal(pathRequest{Path: "Nope", CueFile: "missing.cue"})
	req := httptest.NewRequest(http.MethodPost, "/api/preview", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestHandlePreview_PathTraversal(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	// A CUE file that exists, but outside inputDir, reached via "..".
	outsideDir := t.TempDir()
	writeFile(t, filepath.Join(outsideDir, "escape.cue"), twoTrackCue)
	_ = inputDir

	body, _ := json.Marshal(pathRequest{
		Path:    "..",
		CueFile: filepath.Base(outsideDir) + "/escape.cue",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/preview", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	// filepath.Join cleans ".." against inputDir's parent; the resulting
	// path either misses entirely (404) or resolves outside inputDir (403).
	// Either way it must never succeed.
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 403 or 404", rr.Code)
	}
}

func TestHandlePreview_SymlinkEscapeRejected(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	outsideDir := t.TempDir()
	writeFile(t, filepath.Join(outsideDir, "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(outsideDir, "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	linkPath := filepath.Join(inputDir, "Escape")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	body, _ := json.Marshal(pathRequest{Path: "Escape", CueFile: "album.cue"})
	req := httptest.NewRequest(http.MethodPost, "/api/preview", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestHandleCover_Found(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	writeFile(t, filepath.Join(inputDir, "Album", "cover.jpg"), "fake-jpeg-bytes")

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cover/Album", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body)
	}
	if rr.Body.String() != "fake-jpeg-bytes" {
		t.Fatalf("body = %q, want fake-jpeg-bytes", rr.Body.String())
	}
}

func TestHandleCover_NotFound(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	if err := os.MkdirAll(filepath.Join(inputDir, "Album"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cover/Album", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestHandleCover_PathTraversal(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cover/..%2F..%2Fetc", nil))

	if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 403 or 404", rr.Code)
	}
}

func TestHandleSplit_EnqueueThenDuplicate(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		close(started)
		<-release
		return []string{"01 - First.flac"}, nil
	}
	s, inputDir, _ := testServer(t, splitFn)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	body, _ := json.Marshal(pathRequest{Path: "Album", CueFile: "album.cue"})

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(body)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("first split status = %d, want 202: %s", rr.Code, rr.Body)
	}
	<-started

	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(body)))
	if rr2.Code != http.StatusConflict {
		t.Fatalf("duplicate split status = %d, want 409: %s", rr2.Code, rr2.Body)
	}

	close(release)
}

// A refusal that is not a duplicate must not answer 409: telling the client
// "Already in progress" and handing it a job_id for a job that was never
// created sends it polling an id that only ever 404s.
func TestHandleSplit_RefusedWhenManagerStopped(t *testing.T) {
	inputDir := t.TempDir()
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	// A manager whose context is already cancelled refuses every enqueue with
	// ErrShutdown — the same non-duplicate branch a full queue takes.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr := job.NewManager(ctx, func(ctx context.Context, opts split.Options) ([]string, error) {
		return nil, nil
	})
	cfg := config.Config{InputDir: inputDir, OutputDir: t.TempDir(), Port: 5000}
	s, err := New(cfg, mgr, BuildInfo{App: "test-version", Shntool: "9.9.9"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	body, _ := json.Marshal(pathRequest{Path: "Album", CueFile: "album.cue"})
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(body)))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rr.Code, rr.Body)
	}
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if _, ok := resp["job_id"]; ok {
		t.Fatalf("response carries job_id = %q; no job was created, so there is nothing to poll", resp["job_id"])
	}
}

func TestHandleSplit_OutputEscapeRejected(t *testing.T) {
	// A CUE that legitimately resolves under inputDir, reached via a ".."
	// path whose input-side join stays contained but whose output-side join
	// (filepath.Join(outputDir, path)) would escape OUTPUT_DIR. The request
	// must be rejected before any split is enqueued.
	var captured *split.Options
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		captured = &opts
		return []string{"01 - First.flac"}, nil
	}
	s, inputDir, _ := testServer(t, splitFn)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	// "../<inputBase>/Album": Join(inputDir, ..) climbs to inputDir's parent
	// then back into inputDir/Album (still contained), while Join(outputDir,
	// ..) escapes outputDir into a sibling path.
	traversal := "../" + filepath.Base(inputDir) + "/Album"
	body, _ := json.Marshal(pathRequest{Path: traversal, CueFile: "album.cue"})
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(body)))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if captured != nil {
		t.Fatalf("split was enqueued with OutDir %q, want no split for a traversal path", captured.OutDir)
	}
}

func TestHandleSplit_MissingCue(t *testing.T) {
	s, _, _ := testServer(t, nil)

	body, _ := json.Marshal(pathRequest{Path: "Nope", CueFile: "missing.cue"})
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(body)))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestHandleStatus_Found(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		close(started)
		<-release
		return []string{"01 - First.flac"}, nil
	}
	s, inputDir, _ := testServer(t, splitFn)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	body, _ := json.Marshal(pathRequest{Path: "Album", CueFile: "album.cue"})
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(body)))
	<-started
	close(release)

	deadline := time.Now().Add(time.Second)
	var resp statusResponse
	for {
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/status/Album/album.cue", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body)
		}
		decodeJSON(t, rr, &resp)
		if resp.Status == job.StatusDone || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if resp.Status != job.StatusDone {
		t.Fatalf("job status = %q, want done", resp.Status)
	}
}

// TestHandleStatus_LogSince pins log_since's contract: an absent parameter
// returns the whole retained buffer and its own log_next, and a follow-up
// request with that cursor returns only entries added after it.
func TestHandleStatus_LogSince(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		close(started)
		<-release
		opts.Log(joblog.LevelInfo, "midway")
		return []string{"01 - First.flac"}, nil
	}
	s, inputDir, _ := testServer(t, splitFn)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	body, _ := json.Marshal(pathRequest{Path: "Album", CueFile: "album.cue"})
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(body)))
	<-started

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/status/Album/album.cue", nil))
	var resp statusResponse
	decodeJSON(t, rr, &resp)
	if len(resp.Log) != 1 || resp.Log[0].Text != "starting split" {
		t.Fatalf("log = %+v, want one 'starting split' entry for an absent log_since", resp.Log)
	}
	if resp.LogNext != 1 {
		t.Fatalf("log_next = %d, want 1", resp.LogNext)
	}
	cursor := resp.LogNext

	close(release)

	deadline := time.Now().Add(time.Second)
	for {
		rr = httptest.NewRecorder()
		s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/status/Album/album.cue?log_since=%d", cursor), nil))
		decodeJSON(t, rr, &resp)
		if resp.Status == job.StatusDone || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if resp.Status != job.StatusDone {
		t.Fatalf("job status = %q, want done", resp.Status)
	}
	if len(resp.Log) != 1 || resp.Log[0].Text != "midway" {
		t.Fatalf("tail log = %+v, want only the post-cursor 'midway' entry", resp.Log)
	}
}

// TestHandleStatus_LogEmptyIsArrayNotNull uses the single-worker FIFO queue
// to hold a second job in StatusQueued (log never touched) so its response
// pins the "never null" contract on a genuinely empty buffer.
func TestHandleStatus_LogEmptyIsArrayNotNull(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		close(started)
		<-release
		return []string{"01 - First.flac"}, nil
	}
	s, inputDir, _ := testServer(t, splitFn)
	writeFile(t, filepath.Join(inputDir, "AlbumA", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "AlbumA", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))
	writeFile(t, filepath.Join(inputDir, "AlbumB", "album.cue"), twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "AlbumB", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	bodyA, _ := json.Marshal(pathRequest{Path: "AlbumA", CueFile: "album.cue"})
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(bodyA)))
	<-started // the single worker is now blocked processing AlbumA

	bodyB, _ := json.Marshal(pathRequest{Path: "AlbumB", CueFile: "album.cue"})
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/split", bytes.NewReader(bodyB)))

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/status/AlbumB/album.cue", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body)
	}
	var resp statusResponse
	decodeJSON(t, rr, &resp)
	if resp.Status != job.StatusQueued {
		t.Fatalf("status = %q, want queued", resp.Status)
	}
	if resp.Log == nil || len(resp.Log) != 0 {
		t.Fatalf("log = %#v, want a non-nil empty slice", resp.Log)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"log":[]`)) {
		t.Fatalf("body = %s, want the wire shape \"log\":[]", rr.Body)
	}

	close(release)
}

// TestDecodePathRequest_InvalidBody pins the shared decodePathRequest 400 on
// both endpoints that use it.
func TestDecodePathRequest_InvalidBody(t *testing.T) {
	for _, endpoint := range []string{"/api/preview", "/api/split"} {
		t.Run(endpoint, func(t *testing.T) {
			s, _, _ := testServer(t, nil)

			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader([]byte("{not json")))
			s.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body)
			}
			var body map[string]string
			decodeJSON(t, rr, &body)
			if body["error"] != "invalid request body" {
				t.Fatalf("error = %q, want invalid request body", body["error"])
			}
		})
	}
}

// TestScanRelPath_AbsolutePathRejected covers the filepath.IsAbs arm of
// scanRelPath: an absolute path is refused even when it names a real,
// contained CUE file.
func TestScanRelPath_AbsolutePathRejected(t *testing.T) {
	for _, endpoint := range []string{"/api/preview", "/api/split"} {
		t.Run(endpoint, func(t *testing.T) {
			s, inputDir, _ := testServer(t, nil)
			albumDir := filepath.Join(inputDir, "Album")
			writeFile(t, filepath.Join(albumDir, "album.cue"), twoTrackCue)
			writeFile(t, filepath.Join(albumDir, "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

			body, _ := json.Marshal(pathRequest{Path: albumDir, CueFile: "album.cue"})
			rr := httptest.NewRecorder()
			s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body)))

			if rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body)
			}
		})
	}
}

func TestHandlePreview_ParseError(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	cuePath := filepath.Join(inputDir, "Album", "album.cue")
	writeFile(t, cuePath, twoTrackCue)
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	// cue.Parse only fails when the file cannot be read; the containment
	// checks ahead of it need the file to exist, so make it unreadable.
	if err := os.Chmod(cuePath, 0o000); err != nil {
		t.Fatalf("Chmod(%q): %v", cuePath, err)
	}
	t.Cleanup(func() { _ = os.Chmod(cuePath, 0o644) })
	if _, err := os.ReadFile(cuePath); err == nil {
		t.Skip("file mode does not restrict reads (running as root?)")
	}

	body, _ := json.Marshal(pathRequest{Path: "Album", CueFile: "album.cue"})
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/preview", bytes.NewReader(body)))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rr.Code, rr.Body)
	}
}

// TestHandlePreview_NilTracksNormalized pins the []cue.Track{} normalization:
// a CUE with no TRACK entries must serialize "tracks": [], not null, since the
// SPA maps over the array.
func TestHandlePreview_NilTracksNormalized(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	writeFile(t, filepath.Join(inputDir, "Album", "album.cue"), "PERFORMER \"Artist\"\nTITLE \"Album\"\nFILE \"album.wav\" WAVE\n")
	writeFile(t, filepath.Join(inputDir, "Album", "album.wav"), string(buildWavHeader(44100, 88200, 88200)))

	body, _ := json.Marshal(pathRequest{Path: "Album", CueFile: "album.cue"})
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/preview", bytes.NewReader(body)))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body)
	}

	var raw map[string]json.RawMessage
	decodeJSON(t, rr, &raw)
	if got := string(raw["tracks"]); got != "[]" {
		t.Fatalf("tracks = %s, want []", got)
	}
}

// TestHandleCover_ForbiddenRealPath separates handleCover's 403 (the directory
// resolves outside the input root) from its 404 (contained, but no cover).
func TestHandleCover_ForbiddenRealPath(t *testing.T) {
	s, inputDir, _ := testServer(t, nil)
	outsideDir := t.TempDir()
	writeFile(t, filepath.Join(outsideDir, "cover.jpg"), "fake-jpeg-bytes")

	linkPath := filepath.Join(inputDir, "Escape")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cover/Escape", nil))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (a cover exists there, so a 404 would mean the containment check never ran)", rr.Code)
	}
}

func TestHandleStatus_NotFound(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/status/nope/nope.cue", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// TestParseLogSince pins the documented contract: anything that is not a
// non-negative integer — absent, garbage, or negative — clamps to 0, so a
// malformed client cursor returns the whole buffer rather than reaching
// Buffer.Since with a value it would have to defend against.
func TestParseLogSince(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 0},
		{raw: "0", want: 0},
		{raw: "42", want: 42},
		{raw: "abc", want: 0},
		{raw: "-5", want: 0},
		{raw: "3.5", want: 0},
		{raw: "99999999999999999999", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := parseLogSince(tt.raw); got != tt.want {
				t.Errorf("parseLogSince(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}
