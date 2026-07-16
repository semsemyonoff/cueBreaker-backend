package job

import (
	"context"
	"fmt"
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

	// Closed when the worker leaves its loop. Shutdown is otherwise
	// unobservable, which leaves a test with no way to enqueue *after* the
	// worker is gone: while it still sits in the select, a cancelled ctx and a
	// ready queue are both live cases and the choice between them is random.
	workerDone chan struct{}
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
		splitFn:    splitFn,
		ctx:        ctx,
		jobs:       make(map[string]*State),
		queue:      make(chan queuedJob, 64),
		workerDone: make(chan struct{}),
	}
	go m.worker()
	return m
}

// Enqueue registers a new split job under id and returns true once it is
// queued. It returns false without enqueuing anything if a job with this id
// is already queued, splitting, or tagging, if the queue is full, or if the
// Manager's context is already done.
func (m *Manager) Enqueue(id string, opts split.Options) bool {
	m.mu.Lock()
	if existing, ok := m.jobs[id]; ok && existing.Status.active() {
		m.mu.Unlock()
		return false
	}
	m.jobs[id] = &State{Status: StatusQueued}
	m.mu.Unlock()

	// Checked before the send, not as a second `case` alongside it: with a
	// cancelled ctx and a ready buffer both live, select picks at random, so
	// half the post-shutdown enqueues would be accepted.
	select {
	case <-m.ctx.Done():
		m.reject(id)
		return false
	default:
	}

	jobCtx, cancel := context.WithCancel(m.ctx)
	// The send must never block: one worker drains this queue one split at a
	// time, so a full buffer would park the caller's HTTP handler for minutes
	// with no way to cancel it. Refusing is honest; accepting a job that only
	// a freed slot could ever start is not.
	select {
	case m.queue <- queuedJob{id: id, opts: opts, ctx: jobCtx, cancel: cancel}:
		return true
	default:
		cancel()
		m.reject(id)
		return false
	}
}

// reject drops the StatusQueued placeholder Enqueue optimistically registered,
// so a refused job leaves no state behind for Get to report.
func (m *Manager) reject(id string) {
	m.mu.Lock()
	delete(m.jobs, id)
	m.mu.Unlock()
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
	defer close(m.workerDone)
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

	// A panic in the split pipeline (unexpected tool output, a parsing
	// surprise) must not escape this goroutine and crash the whole server:
	// contain it to this job by marking it errored.
	defer func() {
		if r := recover(); r != nil {
			m.update(qj.id, func(s *State) {
				s.Status = StatusError
				s.Message = fmt.Sprintf("split panicked: %v", r)
			})
		}
	}()

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
