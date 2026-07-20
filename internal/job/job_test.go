package job

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.horn/cueBreaker/backend/internal/joblog"
	"git.horn/cueBreaker/backend/internal/split"
)

func waitForStatus(t *testing.T, m *Manager, id string, want Status, timeout time.Duration) State {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		s, ok := m.Get(id)
		if ok && s.Status == want {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %q: status = %+v, want %v within %v", id, s, want, timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestManager_EnqueueRunDone(t *testing.T) {
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		opts.Progress(0, 4, "Calculating breakpoints...")
		opts.Progress(2, 4, "Splitting complete, tagging...")
		return []string{"01 - Track.flac", "02 - Track.flac"}, nil
	}
	m := NewManager(context.Background(), splitFn)

	if err := m.Enqueue("dir/album.cue", split.Options{}); err != nil {
		t.Fatalf("Enqueue() = %v, want nil", err)
	}

	s := waitForStatus(t, m, "dir/album.cue", StatusDone, time.Second)
	if len(s.ResultFiles) != 2 {
		t.Fatalf("ResultFiles = %v, want 2 entries", s.ResultFiles)
	}
	if s.Message != "Split complete" {
		t.Fatalf("Message = %q, want %q", s.Message, "Split complete")
	}
}

func TestManager_ProgressPhases(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		opts.Progress(1, 4, "Splitting: track 1")
		close(started)
		<-release
		opts.Progress(3, 4, "Tagging: track 1")
		return nil, nil
	}
	m := NewManager(context.Background(), splitFn)
	m.Enqueue("id", split.Options{})

	<-started
	s, _ := m.Get("id")
	if s.Status != StatusSplitting {
		t.Fatalf("Status = %v, want %v (current=1, total=4)", s.Status, StatusSplitting)
	}
	if s.ProgressCurrent != 1 || s.ProgressTotal != 4 {
		t.Fatalf("progress = %d/%d, want 1/4", s.ProgressCurrent, s.ProgressTotal)
	}
	close(release)

	waitForStatus(t, m, "id", StatusDone, time.Second)
}

func TestManager_Enqueue_DuplicateRejectedWhileQueued(t *testing.T) {
	blockFirst := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		closeOnce.Do(func() { close(blockFirst) })
		<-release
		return nil, nil
	}
	m := NewManager(context.Background(), splitFn)

	// A different job occupies the single worker so the second Enqueue for
	// "id" lands in the queue behind it, still in the queued state.
	if err := m.Enqueue("other", split.Options{}); err != nil {
		t.Fatalf("Enqueue(other) = %v, want nil", err)
	}
	<-blockFirst // "other" is now running inside splitFn, worker is busy.

	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue(id) = %v, want nil", err)
	}
	s, ok := m.Get("id")
	if !ok || s.Status != StatusQueued {
		t.Fatalf("Get(id) = %+v, %v; want StatusQueued", s, ok)
	}

	if err := m.Enqueue("id", split.Options{}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Enqueue(id) duplicate = %v, want ErrDuplicate while queued", err)
	}

	close(release)
	waitForStatus(t, m, "id", StatusDone, time.Second)
}

func TestManager_Enqueue_DuplicateRejectedWhileSplitting(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		closeOnce.Do(func() { close(started) })
		<-release
		return nil, nil
	}
	m := NewManager(context.Background(), splitFn)

	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue(id) = %v, want nil", err)
	}
	<-started

	if err := m.Enqueue("id", split.Options{}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Enqueue(id) duplicate = %v, want ErrDuplicate while splitting", err)
	}

	close(release)
	waitForStatus(t, m, "id", StatusDone, time.Second)

	// Once the job is done, its slot is free again.
	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue(id) after done = %v, want nil", err)
	}
}

func TestManager_Serialization(t *testing.T) {
	var mu struct {
		concurrent int
		maxSeen    int
	}
	guard := make(chan struct{}, 1)
	guard <- struct{}{}

	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		<-guard
		mu.concurrent++
		if mu.concurrent > mu.maxSeen {
			mu.maxSeen = mu.concurrent
		}
		guard <- struct{}{}

		time.Sleep(20 * time.Millisecond)

		<-guard
		mu.concurrent--
		guard <- struct{}{}
		return nil, nil
	}
	m := NewManager(context.Background(), splitFn)

	m.Enqueue("a", split.Options{})
	m.Enqueue("b", split.Options{})

	waitForStatus(t, m, "a", StatusDone, time.Second)
	waitForStatus(t, m, "b", StatusDone, time.Second)

	if mu.maxSeen != 1 {
		t.Fatalf("max concurrent splitFn invocations = %d, want 1 (serialized)", mu.maxSeen)
	}
}

func TestManager_ErrorMapping(t *testing.T) {
	wantErr := errors.New("cuebreakpoints failed: bad cue sheet")
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		return nil, wantErr
	}
	m := NewManager(context.Background(), splitFn)
	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue() = %v, want nil", err)
	}

	s := waitForStatus(t, m, "id", StatusError, time.Second)
	if s.Message != wantErr.Error() {
		t.Fatalf("Message = %q, want %q", s.Message, wantErr.Error())
	}

	// split.Run logs nothing for a failure it cannot attribute to a tool, so
	// the manager must put the reason in the log itself — the UI auto-expands
	// this panel precisely on status=error.
	entries, _ := s.Log.Since(0)
	var found bool
	for _, e := range entries {
		if e.Level == joblog.LevelError && strings.Contains(e.Text, wantErr.Error()) {
			found = true
		}
	}
	if !found {
		t.Fatalf("Log entries = %v, want an error entry carrying %q", entries, wantErr)
	}
}

func TestManager_PanicContained(t *testing.T) {
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		panic("unexpected shnsplit output")
	}
	m := NewManager(context.Background(), splitFn)
	if err := m.Enqueue("boom", split.Options{}); err != nil {
		t.Fatalf("Enqueue() = %v, want nil", err)
	}

	// The panicking job is marked errored rather than crashing the worker,
	// and a subsequent job on the same manager still runs to completion.
	s := waitForStatus(t, m, "boom", StatusError, time.Second)
	if s.Message == "" {
		t.Fatalf("Message = %q, want a non-empty panic message", s.Message)
	}

	m.splitFn = func(ctx context.Context, opts split.Options) ([]string, error) {
		return nil, nil
	}
	m.Enqueue("after", split.Options{})
	waitForStatus(t, m, "after", StatusDone, time.Second)
}

// The worker stops once the Manager's context is done. Nothing drains the queue
// after that, so Enqueue must refuse — parking a job at queued forever would
// have the handler answer 202 "queued" for work that can never start, and leave
// a caller polling a status that never moves.
func TestManager_WorkerStopsOnContextDone(t *testing.T) {
	var calls atomic.Int32
	ran := make(chan struct{}, 1)
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		calls.Add(1)
		ran <- struct{}{}
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager(ctx, splitFn)

	// Wait for the worker to actually observe ctx.Done() and leave its loop.
	// A bare sleep races both ways: too short and the worker is still running,
	// so the assertion below is a real failure; too generous and a worker that
	// never started makes `calls == 0` pass vacuously.
	cancel()
	select {
	case <-m.workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("worker still running 2s after its context was cancelled")
	}

	if err := m.Enqueue("id", split.Options{}); !errors.Is(err, ErrShutdown) {
		t.Fatalf("Enqueue() = %v after shutdown, want ErrShutdown — nothing drains the queue", err)
	}

	select {
	case <-ran:
		t.Fatalf("splitFn called %d times, want 0 after the worker shut down", calls.Load())
	case <-time.After(100 * time.Millisecond):
	}

	if s, ok := m.Get("id"); ok {
		t.Fatalf("Get(id) = %+v, true; want not found — a refused job leaves no state", s)
	}
}

// A full queue must refuse rather than park the caller: the sole worker drains
// one split at a time, so a blocking send would hang an HTTP handler for
// minutes with no way to cancel it.
func TestManager_EnqueueRefusesWhenQueueFull(t *testing.T) {
	release := make(chan struct{})
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		<-release
		return nil, nil
	}

	m := NewManager(t.Context(), splitFn)
	defer close(release)

	// Park the worker inside splitFn first. Waiting for `splitting` rather than
	// enqueuing straight through is what makes the count exact: until the worker
	// has taken a job, every send lands in the buffer and the arithmetic below
	// is off by one.
	if err := m.Enqueue("occupier", split.Options{}); err != nil {
		t.Fatalf("Enqueue(occupier) = %v, want nil", err)
	}
	waitForStatus(t, m, "occupier", StatusSplitting, 2*time.Second)

	// With the worker busy, the buffer takes exactly cap(m.queue) more.
	for i := range cap(m.queue) {
		if err := m.Enqueue(fmt.Sprintf("id-%d", i), split.Options{}); err != nil {
			t.Fatalf("Enqueue(id-%d) = %v, want nil — still within capacity", i, err)
		}
	}

	// Enqueue must not block, so the assertion is only reachable if it returns.
	done := make(chan error, 1)
	go func() { done <- m.Enqueue("overflow", split.Options{}) }()

	select {
	case err := <-done:
		if !errors.Is(err, ErrQueueFull) {
			t.Fatalf("Enqueue() = %v with the queue full, want ErrQueueFull", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue() blocked on a full queue — it must refuse instead")
	}

	if s, ok := m.Get("overflow"); ok {
		t.Fatalf("Get(overflow) = %+v, true; want not found — a refused job leaves no state", s)
	}
}

// A refusal must not erase what the id already held: Enqueue overwrites the
// entry with a queued placeholder before it knows the send will succeed, so a
// refused re-split of a finished album would otherwise turn that album's `done`
// into a 404 on the next poll.
func TestManager_EnqueueRefused_KeepsPriorState(t *testing.T) {
	release := make(chan struct{})
	var runs atomic.Int32
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		// The first job completes; every later one parks, so the queue fills.
		if runs.Add(1) > 1 {
			<-release
		}
		return []string{"01 - Track.flac"}, nil
	}

	m := NewManager(t.Context(), splitFn)
	defer close(release)

	// "done-album" runs to completion and keeps its results...
	if err := m.Enqueue("done-album", split.Options{}); err != nil {
		t.Fatalf("Enqueue(done-album) = %v, want nil", err)
	}
	waitForStatus(t, m, "done-album", StatusDone, 2*time.Second)

	// ...then the worker parks on another job and the buffer fills behind it.
	if err := m.Enqueue("occupier", split.Options{}); err != nil {
		t.Fatalf("Enqueue(occupier) = %v, want nil", err)
	}
	waitForStatus(t, m, "occupier", StatusSplitting, 2*time.Second)
	for i := range cap(m.queue) {
		if err := m.Enqueue(fmt.Sprintf("filler-%d", i), split.Options{}); err != nil {
			t.Fatalf("Enqueue(filler-%d) = %v, want nil", i, err)
		}
	}

	if err := m.Enqueue("done-album", split.Options{}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Enqueue(done-album) re-split = %v, want ErrQueueFull", err)
	}

	s, ok := m.Get("done-album")
	if !ok {
		t.Fatal("Get(done-album) = not found; a refused re-split must not delete the completed run")
	}
	if s.Status != StatusDone {
		t.Fatalf("Status = %v, want %v — the refused re-split clobbered the completed state", s.Status, StatusDone)
	}
	if len(s.ResultFiles) != 1 {
		t.Fatalf("ResultFiles = %v, want the previous run's 1 entry", s.ResultFiles)
	}
}

// Re-enqueuing a completed job id re-runs it and replaces its state: an
// album re-split after a CUE fix must not report the previous run's results.
func TestManager_Enqueue_CompletedJobRerunsAndReplacesState(t *testing.T) {
	var runs atomic.Int32
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		if runs.Add(1) == 1 {
			return []string{"01 - Old.flac"}, nil
		}
		return []string{"01 - New.flac", "02 - New.flac"}, nil
	}
	m := NewManager(context.Background(), splitFn)

	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue() = %v, want nil", err)
	}
	first := waitForStatus(t, m, "id", StatusDone, time.Second)
	if len(first.ResultFiles) != 1 {
		t.Fatalf("first run ResultFiles = %v, want 1 entry", first.ResultFiles)
	}

	// A done job holds no slot, so the same id is accepted again.
	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue() after done = %v, want nil", err)
	}

	// Enqueue resets the registry entry to queued, so wait for the second
	// run's distinct result rather than for StatusDone (which the first run
	// already satisfies).
	deadline := time.Now().Add(time.Second)
	for {
		s, _ := m.Get("id")
		if s.Status == StatusDone && len(s.ResultFiles) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job = %+v, want the second run's 2 result files within 1s", s)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := runs.Load(); got != 2 {
		t.Fatalf("splitFn ran %d times, want 2", got)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	m := NewManager(context.Background(), func(ctx context.Context, opts split.Options) ([]string, error) {
		return nil, nil
	})
	if _, ok := m.Get("missing"); ok {
		t.Fatalf("Get(missing) ok = true, want false")
	}
}

func TestManager_Log_CompletedRunReadable(t *testing.T) {
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		opts.Log(joblog.LevelInfo, "cue parsed: 4 tracks")
		opts.Log(joblog.LevelWarn, "removed pregap file: 00 - pregap.flac")
		return []string{"01 - Track.flac"}, nil
	}
	m := NewManager(context.Background(), splitFn)

	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue() = %v, want nil", err)
	}
	s := waitForStatus(t, m, "id", StatusDone, time.Second)

	entries, _ := s.Log.Since(0)
	if len(entries) < 3 {
		t.Fatalf("Log entries = %v, want at least 3 (starting split + 2 emitted)", entries)
	}
	if entries[0].Text != "starting split" {
		t.Fatalf("entries[0].Text = %q, want %q", entries[0].Text, "starting split")
	}
	var sawParsed, sawPregap bool
	for _, e := range entries {
		switch e.Text {
		case "cue parsed: 4 tracks":
			sawParsed = e.Level == joblog.LevelInfo
		case "removed pregap file: 00 - pregap.flac":
			sawPregap = e.Level == joblog.LevelWarn
		}
	}
	if !sawParsed || !sawPregap {
		t.Fatalf("Log entries = %v, missing expected lines", entries)
	}
}

func TestManager_Log_RerunStartsEmpty(t *testing.T) {
	var runs atomic.Int32
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		if runs.Add(1) == 1 {
			opts.Log(joblog.LevelInfo, "first run line")
			return []string{"01 - Old.flac"}, nil
		}
		return []string{"01 - New.flac"}, nil
	}
	m := NewManager(context.Background(), splitFn)

	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue() = %v, want nil", err)
	}
	waitForStatus(t, m, "id", StatusDone, time.Second)

	if err := m.Enqueue("id", split.Options{}); err != nil {
		t.Fatalf("Enqueue() rerun = %v, want nil", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		s, _ := m.Get("id")
		if s.Status == StatusDone && len(s.ResultFiles) == 1 && s.ResultFiles[0] == "01 - New.flac" {
			entries, _ := s.Log.Since(0)
			for _, e := range entries {
				if e.Text == "first run line" {
					t.Fatalf("Log entries = %v, want no carryover from the first run", entries)
				}
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job = %+v, want the second run to complete within 1s", s)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestManager_Log_RejectedEnqueueKeepsPriorLog(t *testing.T) {
	release := make(chan struct{})
	var runs atomic.Int32
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		if runs.Add(1) > 1 {
			<-release
		}
		opts.Log(joblog.LevelInfo, "done-album line")
		return []string{"01 - Track.flac"}, nil
	}
	m := NewManager(t.Context(), splitFn)
	defer close(release)

	if err := m.Enqueue("done-album", split.Options{}); err != nil {
		t.Fatalf("Enqueue(done-album) = %v, want nil", err)
	}
	waitForStatus(t, m, "done-album", StatusDone, 2*time.Second)
	before, _ := m.Get("done-album")
	beforeEntries, _ := before.Log.Since(0)
	if len(beforeEntries) == 0 {
		t.Fatal("done-album log = empty, want entries from its completed run")
	}

	if err := m.Enqueue("occupier", split.Options{}); err != nil {
		t.Fatalf("Enqueue(occupier) = %v, want nil", err)
	}
	waitForStatus(t, m, "occupier", StatusSplitting, 2*time.Second)
	for i := range cap(m.queue) {
		if err := m.Enqueue(fmt.Sprintf("filler-%d", i), split.Options{}); err != nil {
			t.Fatalf("Enqueue(filler-%d) = %v, want nil", i, err)
		}
	}

	if err := m.Enqueue("done-album", split.Options{}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Enqueue(done-album) re-split = %v, want ErrQueueFull", err)
	}

	after, ok := m.Get("done-album")
	if !ok {
		t.Fatal("Get(done-album) = not found; a refused re-split must not delete the completed run")
	}
	afterEntries, _ := after.Log.Since(0)
	if len(afterEntries) != len(beforeEntries) {
		t.Fatalf("Log entries after refused re-split = %v, want unchanged %v", afterEntries, beforeEntries)
	}
}

func TestManager_Log_PanicRecordedAsError(t *testing.T) {
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		panic("unexpected shnsplit output")
	}
	m := NewManager(context.Background(), splitFn)
	if err := m.Enqueue("boom", split.Options{}); err != nil {
		t.Fatalf("Enqueue() = %v, want nil", err)
	}

	s := waitForStatus(t, m, "boom", StatusError, time.Second)
	entries, _ := s.Log.Since(0)
	var sawPanic bool
	for _, e := range entries {
		if e.Level == joblog.LevelError && e.Text == "split panicked: unexpected shnsplit output" {
			sawPanic = true
		}
	}
	if !sawPanic {
		t.Fatalf("Log entries = %v, want an error entry with the panic text", entries)
	}
}

func TestJobID(t *testing.T) {
	if got, want := JobID("Artist/Album", "album.cue"), "Artist/Album/album.cue"; got != want {
		t.Fatalf("JobID() = %q, want %q", got, want)
	}
}
