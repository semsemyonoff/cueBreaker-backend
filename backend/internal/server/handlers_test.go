package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
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

	s, err := New(cfg, mgr, "test-version", logger)
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

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/scan", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body)
	}
	var pairs []scan.Pair
	decodeJSON(t, rr, &pairs)
	if len(pairs) != 1 || pairs[0].Path != "Album" {
		t.Fatalf("pairs = %+v, want one entry for Album", pairs)
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

func TestHandleStatus_NotFound(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/status/nope/nope.cue", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}
