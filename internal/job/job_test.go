package job

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

	if ok := m.Enqueue("dir/album.cue", split.Options{}); !ok {
		t.Fatalf("Enqueue() = false, want true")
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
	if !m.Enqueue("other", split.Options{}) {
		t.Fatalf("Enqueue(other) = false, want true")
	}
	<-blockFirst // "other" is now running inside splitFn, worker is busy.

	if !m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue(id) = false, want true")
	}
	s, ok := m.Get("id")
	if !ok || s.Status != StatusQueued {
		t.Fatalf("Get(id) = %+v, %v; want StatusQueued", s, ok)
	}

	if m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue(id) duplicate = true, want false while queued")
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

	if !m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue(id) = false, want true")
	}
	<-started

	if m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue(id) duplicate = true, want false while splitting")
	}

	close(release)
	waitForStatus(t, m, "id", StatusDone, time.Second)

	// Once the job is done, its slot is free again.
	if !m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue(id) after done = false, want true")
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
	m.Enqueue("id", split.Options{})

	s := waitForStatus(t, m, "id", StatusError, time.Second)
	if s.Message != wantErr.Error() {
		t.Fatalf("Message = %q, want %q", s.Message, wantErr.Error())
	}
}

func TestManager_PanicContained(t *testing.T) {
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		panic("unexpected shnsplit output")
	}
	m := NewManager(context.Background(), splitFn)
	m.Enqueue("boom", split.Options{})

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

// The worker stops once the Manager's context is done. A job enqueued after
// shutdown is registered but never runs — it must sit at queued rather than
// silently start on a canceled context, so a caller polling its status sees
// no progress instead of a bogus done.
func TestManager_WorkerStopsOnContextDone(t *testing.T) {
	var calls atomic.Int32
	splitFn := func(ctx context.Context, opts split.Options) ([]string, error) {
		calls.Add(1)
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager(ctx, splitFn)

	cancel()
	// Give the worker time to observe ctx.Done() and return before anything
	// reaches the queue.
	time.Sleep(50 * time.Millisecond)

	if !m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue() = false, want true — the registry still accepts the job")
	}
	time.Sleep(50 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Fatalf("splitFn called %d times, want 0 after the worker shut down", got)
	}
	s, ok := m.Get("id")
	if !ok || s.Status != StatusQueued {
		t.Fatalf("Get(id) = %+v, %v; want StatusQueued — the job never ran", s, ok)
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

	if !m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue() = false, want true")
	}
	first := waitForStatus(t, m, "id", StatusDone, time.Second)
	if len(first.ResultFiles) != 1 {
		t.Fatalf("first run ResultFiles = %v, want 1 entry", first.ResultFiles)
	}

	// A done job holds no slot, so the same id is accepted again.
	if !m.Enqueue("id", split.Options{}) {
		t.Fatalf("Enqueue() after done = false, want true")
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

func TestJobID(t *testing.T) {
	if got, want := JobID("Artist/Album", "album.cue"), "Artist/Album/album.cue"; got != want {
		t.Fatalf("JobID() = %q, want %q", got, want)
	}
}
