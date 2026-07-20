package split

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.horn/cueBreaker/backend/internal/joblog"
)

// logCall is one recorded Options.Log invocation, formatted the way the
// real joblog.Buffer would format it.
type logCall struct {
	level joblog.Level
	text  string
}

func collectLog(calls *[]logCall) LogFunc {
	return func(level joblog.Level, format string, args ...any) {
		*calls = append(*calls, logCall{level: level, text: fmt.Sprintf(format, args...)})
	}
}

const twoTrackCue = `PERFORMER "Test Artist"
TITLE "Test Album"
FILE "album.flac" WAVE
  TRACK 01 AUDIO
    TITLE "First Track"
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    TITLE "Second Track"
    INDEX 01 03:45:20
`

// writeFakeTool writes an executable shell script named name into dir and
// prepends dir to PATH for the duration of the test, so Run exercises the
// real exec.CommandContext plumbing without depending on the real
// cuebreakpoints/shnsplit binaries being installed.
func writeFakeTool(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}

func setupSource(t *testing.T) (sourceDir, cuePath, outDir string) {
	t.Helper()
	sourceDir = t.TempDir()
	cuePath = filepath.Join(sourceDir, "album.cue")
	if err := os.WriteFile(cuePath, []byte(twoTrackCue), 0o644); err != nil {
		t.Fatalf("write cue fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "album.flac"), []byte("not real audio"), 0o644); err != nil {
		t.Fatalf("write flac fixture: %v", err)
	}
	outDir = filepath.Join(t.TempDir(), "out")
	return sourceDir, cuePath, outDir
}

func TestRun_NoSourceFLAC(t *testing.T) {
	sourceDir := t.TempDir()
	cuePath := filepath.Join(sourceDir, "album.cue")
	if err := os.WriteFile(cuePath, []byte(twoTrackCue), 0o644); err != nil {
		t.Fatalf("write cue fixture: %v", err)
	}
	// No album.flac written: SourceFLAC resolution must fail.

	_, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    filepath.Join(sourceDir, "out"),
	})
	if err == nil || !strings.Contains(err.Error(), "no source FLAC") {
		t.Fatalf("Run() error = %v, want no-source-FLAC error", err)
	}
}

// Run makes a UTF-8 temp copy of the CUE before it touches any tool. When
// the temp file cannot be created the pipeline must abort there — nothing
// downstream is safe to run against the original, possibly non-UTF-8 CUE.
func TestRun_MakeUTF8CueFails(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	// os.CreateTemp resolves its dir through os.TempDir(), which reads
	// $TMPDIR: point it at a path that does not exist.
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "does_not_exist"))

	_, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
	})
	if err == nil || !strings.Contains(err.Error(), "make utf8 cue") {
		t.Fatalf("Run() error = %v, want make-utf8-cue failure", err)
	}
	if _, statErr := os.Stat(outDir); !os.IsNotExist(statErr) {
		t.Fatalf("Run() created OutDir despite aborting earlier: %v", statErr)
	}
}

// The output directory is created before cuebreakpoints runs, so a
// non-creatable OutDir must fail the job rather than let shnsplit write
// somewhere unexpected.
func TestRun_MkdirOutDirFails(t *testing.T) {
	sourceDir, cuePath, _ := setupSource(t)
	// album.flac is a regular file, so nesting a directory under it fails
	// with ENOTDIR.
	outDir := filepath.Join(sourceDir, "album.flac", "out")

	_, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
	})
	if err == nil || !strings.Contains(err.Error(), "create output dir") {
		t.Fatalf("Run() error = %v, want create-output-dir failure", err)
	}
}

func TestRun_CuebreakpointsFails(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	toolDir := t.TempDir()
	writeFakeTool(t, toolDir, "cuebreakpoints", `echo "bad cue sheet" >&2; exit 1`)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
	})
	if err == nil || !strings.Contains(err.Error(), "cuebreakpoints failed") {
		t.Fatalf("Run() error = %v, want cuebreakpoints failure", err)
	}
	if !strings.Contains(err.Error(), "bad cue sheet") {
		t.Fatalf("Run() error = %v, want tool stderr surfaced", err)
	}
}

func TestRun_Success_ProgressCapped(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	toolDir := t.TempDir()
	writeFakeTool(t, toolDir, "cuebreakpoints", `exit 0`)
	writeFakeTool(t, toolDir, "shnsplit", `
outdir=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -d) outdir="$2"; shift 2 ;;
    *) shift ;;
  esac
done
touch "$outdir/00 - pregap.flac"
touch "$outdir/01 - First Track.flac"
touch "$outdir/02 - Second Track.flac"
echo "Splitting [album.flac] --> [00 - pregap.flac] : 100% OK" >&2
echo "Splitting [album.flac] --> [01 - First Track.flac] : 100% OK" >&2
echo "Splitting [album.flac] --> [02 - Second Track.flac] : 100% OK" >&2
exit 0
`)
	writeFakeTool(t, toolDir, "cueprint", `echo tagvalue`)
	writeFakeTool(t, toolDir, "metaflac", `exit 0`)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	type call struct {
		current, total int
		detail         string
	}
	var calls []call
	result, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
		Progress: func(current, total int, detail string) {
			calls = append(calls, call{current, total, detail})
		},
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if info, statErr := os.Stat(outDir); statErr != nil || !info.IsDir() {
		t.Fatalf("Run() did not create OutDir: %v", statErr)
	}

	wantResult := []string{"01 - First Track.flac", "02 - Second Track.flac"}
	if len(result) != len(wantResult) {
		t.Fatalf("result = %v, want %v", result, wantResult)
	}
	for i, name := range wantResult {
		if result[i] != name {
			t.Fatalf("result = %v, want %v", result, wantResult)
		}
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "00 - pregap.flac")); !os.IsNotExist(statErr) {
		t.Fatalf("pregap file was not removed")
	}

	const trackCount = 2
	const totalSteps = trackCount * 2
	for _, c := range calls {
		if c.total != totalSteps {
			t.Fatalf("call %+v: total = %d, want %d", c, c.total, totalSteps)
		}
		if c.current > totalSteps {
			t.Fatalf("call %+v: current exceeds totalSteps %d", c, totalSteps)
		}
	}
	last := calls[len(calls)-1]
	if last.current != totalSteps {
		t.Fatalf("final current = %d, want %d", last.current, totalSteps)
	}
}

// writeSuccessTools writes fake cuebreakpoints (given breakpointsScript),
// shnsplit, cueprint and metaflac binaries onto toolDir, reproducing a
// clean two-track split (one pregap + two real tracks) with no cover.
func writeSuccessTools(t *testing.T, toolDir, breakpointsScript string) {
	t.Helper()
	writeFakeTool(t, toolDir, "cuebreakpoints", breakpointsScript)
	writeFakeTool(t, toolDir, "shnsplit", `
outdir=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -d) outdir="$2"; shift 2 ;;
    *) shift ;;
  esac
done
touch "$outdir/00 - pregap.flac"
touch "$outdir/01 - First Track.flac"
touch "$outdir/02 - Second Track.flac"
echo "Splitting [album.flac] --> [00 - pregap.flac] : 100% OK" >&2
echo "Splitting [album.flac] --> [01 - First Track.flac] : 100% OK" >&2
echo "Splitting [album.flac] --> [02 - Second Track.flac] : 100% OK" >&2
exit 0
`)
	writeFakeTool(t, toolDir, "cueprint", `echo tagvalue`)
	writeFakeTool(t, toolDir, "metaflac", `exit 0`)
}

// TestRun_EmitsEventSequence asserts the full, ordered synthesized event
// log for a clean run: cue/source info, the cuebreakpoints count, one
// "track"/"pregap" line per shnsplit step, the per-track tagging lines,
// the pregap removal warning, the no-cover warning and the closing "done"
// line.
func TestRun_EmitsEventSequence(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	toolDir := t.TempDir()
	writeSuccessTools(t, toolDir, `echo "bp 1"; echo "bp 2"; exit 0`)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var calls []logCall
	result, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
		Log:       collectLog(&calls),
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("result = %v, want 2 files", result)
	}

	want := []logCall{
		{joblog.LevelInfo, `cue parsed: 2 tracks · "Test Album"`},
		{joblog.LevelInfo, "source: album.flac"},
		{joblog.LevelInfo, "cuebreakpoints: 2 breakpoints"},
		{joblog.LevelInfo, "pregap → 00 - pregap.flac"},
		{joblog.LevelInfo, "track 1/2 → 01 - First Track.flac"},
		{joblog.LevelInfo, "track 2/2 → 02 - Second Track.flac"},
		{joblog.LevelInfo, "tagged 1/2: 01 - First Track.flac"},
		{joblog.LevelInfo, "tagged 2/2: 02 - Second Track.flac"},
		{joblog.LevelWarn, "removed pregap file: 00 - pregap.flac"},
		{joblog.LevelWarn, "no cover found"},
		{joblog.LevelInfo, "done: 2 files"},
	}
	if len(calls) != len(want) {
		t.Fatalf("log = %+v, want %+v", calls, want)
	}
	for i, w := range want {
		if calls[i] != w {
			t.Fatalf("log[%d] = %+v, want %+v (full log: %+v)", i, calls[i], w, calls)
		}
	}
}

// TestRun_ShnsplitFails_LogsError asserts that a failing shnsplit produces
// an error-level log entry carrying its stderr, in addition to the error
// it already returns.
func TestRun_ShnsplitFails_LogsError(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	toolDir := t.TempDir()
	writeFakeTool(t, toolDir, "cuebreakpoints", `exit 0`)
	writeFakeTool(t, toolDir, "shnsplit", `echo "possibly corrupt file" >&2; exit 1`)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var calls []logCall
	_, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
		Log:       collectLog(&calls),
	})
	if err == nil {
		t.Fatal("Run() error = nil, want shnsplit failure")
	}

	var found *logCall
	for i := range calls {
		if calls[i].level == joblog.LevelError {
			found = &calls[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("log = %+v, want an error entry", calls)
	}
	if !strings.Contains(found.text, "shnsplit failed") || !strings.Contains(found.text, "possibly corrupt file") {
		t.Fatalf("error log entry = %q, want it to mention the shnsplit failure and its stderr", found.text)
	}
}

// TestRun_NilLog_Unchanged asserts that leaving Options.Log nil runs the
// pipeline exactly as it would with a logger attached — the reporter's log
// methods must no-op rather than panic.
func TestRun_NilLog_Unchanged(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	toolDir := t.TempDir()
	writeSuccessTools(t, toolDir, `exit 0`)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	want := []string{"01 - First Track.flac", "02 - Second Track.flac"}
	if len(result) != len(want) {
		t.Fatalf("result = %v, want %v", result, want)
	}
	for i, name := range want {
		if result[i] != name {
			t.Fatalf("result = %v, want %v", result, want)
		}
	}
}

func TestRun_ContextCanceledKillsShnsplit(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	toolDir := t.TempDir()
	writeFakeTool(t, toolDir, "cuebreakpoints", `exit 0`)
	writeFakeTool(t, toolDir, "shnsplit", `sleep 30`)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, Options{CuePath: cuePath, SourceDir: sourceDir, OutDir: outDir})
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("Run() error = nil, want error after context cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation; shnsplit was not killed")
	}
}

func TestRun_ShnsplitFails(t *testing.T) {
	sourceDir, cuePath, outDir := setupSource(t)
	toolDir := t.TempDir()
	writeFakeTool(t, toolDir, "cuebreakpoints", `exit 0`)
	writeFakeTool(t, toolDir, "shnsplit", `echo "possibly corrupt file" >&2; exit 1`)
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := Run(context.Background(), Options{
		CuePath:   cuePath,
		SourceDir: sourceDir,
		OutDir:    outDir,
	})
	if err == nil || !strings.Contains(err.Error(), "shnsplit failed") {
		t.Fatalf("Run() error = %v, want shnsplit failure", err)
	}
	if !strings.Contains(err.Error(), "possibly corrupt file") {
		t.Fatalf("Run() error = %v, want tool stderr surfaced", err)
	}
}
