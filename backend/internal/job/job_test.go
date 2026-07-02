package job

import (
	"context"
	"errors"
	"sync"
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
