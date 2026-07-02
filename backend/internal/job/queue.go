package job

import (
	"context"
	"sync"

	"git.horn/cueBreaker/backend/internal/split"
)

// SplitFunc matches split.Run's signature, letting tests substitute a fake
// pipeline without shelling out to real tools.
type SplitFunc func(ctx context.Context, opts split.Options) ([]string, error)

// queuedJob is one Enqueue call's payload, carrying the job's own
// cancelable context through to the worker.
type queuedJob struct {
	id     string
	opts   split.Options
	ctx    context.Context
	cancel context.CancelFunc
}

// Manager serializes split jobs: only one runs at a time, in FIFO enqueue
// order, drained by a single worker goroutine. All job state is kept
// in-memory, keyed by job ID (see JobID).
type Manager struct {
	splitFn SplitFunc
	ctx     context.Context

	mu   sync.Mutex
	jobs map[string]*State

	queue chan queuedJob
}

// NewManager starts a Manager and its worker goroutine. ctx bounds the
// lifetime of every job's context: canceling it cancels any job currently
// running and stops the worker once it next reaches for the queue.
// splitFn defaults to split.Run when nil.
func NewManager(ctx context.Context, splitFn SplitFunc) *Manager {
	if splitFn == nil {
		splitFn = split.Run
	}
	m := &Manager{
		splitFn: splitFn,
		ctx:     ctx,
		jobs:    make(map[string]*State),
		queue:   make(chan queuedJob, 64),
	}
	go m.worker()
	return m
}

// Enqueue registers a new split job under id and returns true once it is
// queued. It returns false without enqueuing anything if a job with this id
// is already queued, splitting, or tagging.
func (m *Manager) Enqueue(id string, opts split.Options) bool {
	m.mu.Lock()
	if existing, ok := m.jobs[id]; ok && existing.Status.active() {
		m.mu.Unlock()
		return false
	}
	m.jobs[id] = &State{Status: StatusQueued}
	m.mu.Unlock()

	jobCtx, cancel := context.WithCancel(m.ctx)
	m.queue <- queuedJob{id: id, opts: opts, ctx: jobCtx, cancel: cancel}
	return true
}

// Get returns a copy of job id's current state, and whether it exists.
func (m *Manager) Get(id string) (State, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.jobs[id]
	if !ok {
		return State{}, false
	}
	return *s, true
}

func (m *Manager) worker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case qj := <-m.queue:
			m.run(qj)
		}
	}
}

func (m *Manager) run(qj queuedJob) {
	defer qj.cancel()

	m.update(qj.id, func(s *State) {
		s.Status = StatusSplitting
		s.Message = "Starting split..."
	})

	opts := qj.opts
	opts.Progress = m.progressFunc(qj.id)

	result, err := m.splitFn(qj.ctx, opts)
	if err != nil {
		m.update(qj.id, func(s *State) {
			s.Status = StatusError
			s.Message = err.Error()
		})
		return
	}

	m.update(qj.id, func(s *State) {
		s.Status = StatusDone
		s.Message = "Split complete"
		s.ResultFiles = result
	})
}

// progressFunc adapts split.Run's progress callback into a State update for
// job id, deriving the coarse splitting/tagging phase from how far current
// is into the combined split+tag step count.
func (m *Manager) progressFunc(id string) split.ProgressFunc {
	return func(current, total int, detail string) {
		status := StatusSplitting
		if total > 0 && current >= total/2 {
			status = StatusTagging
		}
		m.update(id, func(s *State) {
			s.Status = status
			s.Message = detail
			s.ProgressCurrent = current
			s.ProgressTotal = total
			s.ProgressDetail = detail
		})
	}
}

func (m *Manager) update(id string, fn func(*State)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.jobs[id]
	if !ok {
		return
	}
	fn(s)
}
