package split

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"git.horn/cueBreaker/backend/internal/cue"
	"git.horn/cueBreaker/backend/internal/joblog"
)

// breakpointsTimeout bounds how long cuebreakpoints may run, mirroring
// app.py's subprocess timeout=30. shnsplit has no such timeout: it runs
// for as long as the job's context allows, and is killed on cancellation.
const breakpointsTimeout = 30 * time.Second

// ProgressFunc receives split progress as the pipeline advances. total is
// the combined split+tag step count (2 * track count).
type ProgressFunc func(current, total int, detail string)

// LogFunc receives synthesized pipeline events as the split runs.
type LogFunc func(level joblog.Level, format string, args ...any)

// Options configures a split run.
type Options struct {
	// CuePath is the path to the original (possibly non-UTF-8) CUE file.
	CuePath string
	// SourceDir is the directory containing CuePath and the source audio.
	SourceDir string
	// OutDir is the destination directory for the split track files.
	OutDir string
	// Progress, if non-nil, is called as the pipeline advances.
	Progress ProgressFunc
	// Log, if non-nil, is called with synthesized pipeline events.
	Log LogFunc
}

// reporter bundles the progress and log callbacks threaded through the
// split pipeline so runShnsplit and finishSplit take one value instead of
// a growing parameter list. Each method is a no-op when its underlying
// callback is nil.
type reporter struct {
	progress ProgressFunc
	log      LogFunc
}

func (r reporter) step(current, total int, detail string) {
	if r.progress != nil {
		r.progress(current, total, detail)
	}
}

func (r reporter) info(format string, args ...any) {
	if r.log != nil {
		r.log(joblog.LevelInfo, format, args...)
	}
}

func (r reporter) warn(format string, args ...any) {
	if r.log != nil {
		r.log(joblog.LevelWarn, format, args...)
	}
}

// maxToolDiagnosticLines caps how much of a failing tool's output is carried
// into the error (and from there into the job log). The useful diagnostic is
// always at the tail; the head is progress chatter, and joblog strips the
// newlines, so an uncapped join renders as one unreadable megaline that is
// re-serialised on every status poll.
const maxToolDiagnosticLines = 10

// toolDiagnostic trims a failing tool's output to its last few non-blank
// lines, prefixing an ellipsis when anything was dropped.
func toolDiagnostic(out string) string {
	var kept []string
	for line := range strings.SplitSeq(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) > maxToolDiagnosticLines {
		kept = append([]string{"…"}, kept[len(kept)-maxToolDiagnosticLines:]...)
	}
	return strings.Join(kept, "; ")
}

// trackFraction formats n/total zero-padded to total's own digit width,
// e.g. trackFraction(3, 14) is "03/14".
func trackFraction(n, total int) string {
	width := len(strconv.Itoa(total))
	return fmt.Sprintf("%0*d/%0*d", width, n, width, total)
}

// countNonEmptyLines counts non-blank lines in s, used to turn
// cuebreakpoints' stdout into a breakpoint count without depending on its
// exact format.
func countNonEmptyLines(s string) int {
	n := 0
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// Run executes the full split pipeline: it resolves the source audio file,
// makes a UTF-8 temp copy of the CUE (removed on every exit path), runs
// cuebreakpoints, then shnsplit — streaming its stderr into Options.Progress,
// capped so it never exceeds the real track count — then tags every real
// track, removes the discarded pregap file, and copies a discovered cover
// into OutDir. It returns the sorted list of resulting FLAC file names.
func Run(ctx context.Context, opts Options) ([]string, error) {
	r := reporter{progress: opts.Progress, log: opts.Log}

	album, err := cue.Parse(opts.CuePath)
	if err != nil {
		return nil, fmt.Errorf("split: parse cue: %w", err)
	}
	trackCount := len(album.Tracks)
	r.info("cue parsed: %d tracks · %q", trackCount, album.Title)

	sourcePath, ok := cue.SourceFLAC(album, opts.SourceDir)
	if !ok {
		return nil, fmt.Errorf("split: no source FLAC/WAV found in %s", opts.SourceDir)
	}
	r.info("source: %s", filepath.Base(sourcePath))

	utf8Cue, err := cue.MakeUTF8Cue(opts.CuePath)
	if err != nil {
		return nil, fmt.Errorf("split: make utf8 cue: %w", err)
	}
	defer func() { _ = os.Remove(utf8Cue) }()

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("split: create output dir: %w", err)
	}

	totalSteps := trackCount * 2

	r.step(0, totalSteps, "Calculating breakpoints...")
	breakpointsOut, err := runCuebreakpoints(ctx, utf8Cue)
	if err != nil {
		return nil, err
	}
	r.info("cuebreakpoints: %d breakpoints", countNonEmptyLines(breakpointsOut))

	r.step(0, totalSteps, "Splitting FLAC...")
	if err := runShnsplit(ctx, utf8Cue, sourcePath, opts.OutDir, trackCount, totalSteps, r); err != nil {
		return nil, err
	}

	r.step(trackCount, totalSteps, "Splitting complete, tagging...")
	result, err := finishSplit(ctx, utf8Cue, album, opts.SourceDir, opts.OutDir, trackCount, totalSteps, r)
	if err != nil {
		return nil, err
	}

	r.step(totalSteps, totalSteps, "Complete")
	r.info("done: %d files", len(result))
	return result, nil
}

// runContext runs name with args under ctx, bounded additionally by
// timeout when timeout > 0. It returns the combined stdout+stderr text so
// callers can surface a failing tool's own diagnostics verbatim.
func runContext(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

func runCuebreakpoints(ctx context.Context, utf8Cue string) (string, error) {
	out, err := runContext(ctx, breakpointsTimeout, "cuebreakpoints", utf8Cue)
	if err != nil {
		// No log entry here: Run's error propagates to the job manager, which
		// logs it once. Logging it here too would double every tool failure.
		return "", fmt.Errorf("split: cuebreakpoints failed: %s", toolDiagnostic(out))
	}
	return out, nil
}

// runShnsplit runs shnsplit directly under ctx (no additional timeout, so
// a long split is only bounded by the job's own cancellation), streaming
// its stderr line-by-line into progress via streamShnsplitProgress.
func runShnsplit(ctx context.Context, utf8Cue, sourcePath, outDir string, trackCount, totalSteps int, r reporter) error {
	cmd := exec.CommandContext(ctx, "shnsplit",
		"-f", utf8Cue,
		"-O", "always",
		"-o", "flac",
		"-t", "%n - %t",
		"-d", outDir,
		sourcePath,
	)
	// Run shnsplit in its own process group and, on cancellation, SIGKILL the
	// whole group. shnsplit spawns encoder children; the default per-process
	// kill would orphan them, and an orphan that inherited the stderr pipe keeps
	// it open — the synchronous read below would then block until it exits.
	// WaitDelay is a backstop that force-closes the pipes shortly after.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("split: shnsplit stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("split: shnsplit start: %w", err)
	}

	realTrackNum := 0
	lines, readErr := streamShnsplitProgress(stderr, trackCount, func(step splitStep) {
		r.step(step.current, totalSteps, "Splitting: "+step.detail)
		if isPregapFile(step.detail) {
			r.info("pregap → %s", step.detail)
		} else {
			// step.current is shared with the pregap line and capped at
			// trackCount, so it cannot be used as this track's own number
			// (the pregap line would otherwise consume a slot and shift or
			// duplicate the real numbering). Count real tracks separately.
			realTrackNum++
			r.info("track %s → %s", trackFraction(realTrackNum, trackCount), step.detail)
		}
	})

	waitErr := cmd.Wait()
	if readErr != nil {
		return fmt.Errorf("split: read shnsplit stderr: %w", readErr)
	}
	if waitErr != nil {
		// As in runCuebreakpoints: the job manager logs the returned error once.
		return fmt.Errorf("split: shnsplit failed: %s", toolDiagnostic(strings.Join(lines, "\n")))
	}
	return nil
}
