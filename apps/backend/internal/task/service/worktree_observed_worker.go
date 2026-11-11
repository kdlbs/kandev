package service

import (
	"context"

	"github.com/kandev/kandev/internal/events"
	"sync"
)

// worktreeObservationWorker coalesces linked-worktree snapshots per task
// environment. Event-bus delivery only enqueues; git inspection, database
// writes, and agentctl rescans run on this worker instead of the workspace
// stream goroutine.
type worktreeObservationWorker struct {
	mu      sync.Mutex
	latest  map[string]*events.WorktreeInventoryData
	wake    chan struct{}
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	process func(context.Context, *events.WorktreeInventoryData)
}

func newWorktreeObservationWorker(process func(context.Context, *events.WorktreeInventoryData)) *worktreeObservationWorker {
	return &worktreeObservationWorker{
		latest:  make(map[string]*events.WorktreeInventoryData),
		wake:    make(chan struct{}, 1),
		process: process,
	}
}

func (w *worktreeObservationWorker) start(parent context.Context) {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.started = true
	w.wg.Add(1)
	w.mu.Unlock()
	go w.run(ctx)
}

func (w *worktreeObservationWorker) stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	cancel := w.cancel
	w.cancel = nil
	w.started = false
	w.latest = make(map[string]*events.WorktreeInventoryData)
	w.mu.Unlock()
	cancel()
	w.wg.Wait()
}

func (w *worktreeObservationWorker) enqueue(payload *events.WorktreeInventoryData) {
	if payload == nil {
		return
	}
	key := payload.TaskEnvironmentID
	if key == "" {
		key = payload.TaskID
	}
	if key == "" {
		return
	}
	w.mu.Lock()
	w.latest[key] = payload
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *worktreeObservationWorker) drain() []*events.WorktreeInventoryData {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.latest) == 0 {
		return nil
	}
	pending := make([]*events.WorktreeInventoryData, 0, len(w.latest))
	for key, payload := range w.latest {
		pending = append(pending, payload)
		delete(w.latest, key)
	}
	return pending
}

func (w *worktreeObservationWorker) run(ctx context.Context) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
			for _, payload := range w.drain() {
				if ctx.Err() != nil {
					return
				}
				w.process(ctx, payload)
			}
		}
	}
}

func (s *Service) worktreeObservationWorker() *worktreeObservationWorker {
	s.worktreeObservationMu.Lock()
	defer s.worktreeObservationMu.Unlock()
	if s.worktreeObservations == nil {
		s.worktreeObservations = newWorktreeObservationWorker(s.processWorkspaceWorktreesObserved)
	}
	return s.worktreeObservations
}

// StartWorktreeAdoptionWorker starts the coalescing worker. It is idempotent
// and stops when the supplied context is cancelled or Stop is called.
func (s *Service) StartWorktreeAdoptionWorker(ctx context.Context) {
	s.worktreeObservationWorker().start(ctx)
}

// StopWorktreeAdoptionWorker drains no queued work and waits for the running
// observation, if any, to finish.
func (s *Service) StopWorktreeAdoptionWorker() {
	s.worktreeObservationMu.Lock()
	worker := s.worktreeObservations
	s.worktreeObservationMu.Unlock()
	if worker != nil {
		worker.stop()
	}
}
