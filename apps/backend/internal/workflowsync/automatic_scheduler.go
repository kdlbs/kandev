package workflowsync

import (
	"context"
	"sync"
)

// automaticSyncWorkerLimit is deliberately small. Automatic sync is a
// background activity and a provider admission wait must not turn every due
// workspace into a goroutine.
const automaticSyncWorkerLimit = 4

type automaticJob struct {
	workspaceID string
	force       bool
	state       *automaticJobState
}

type automaticJobState struct {
	mu           sync.Mutex
	force        bool
	generation   uint64
	cancelled    bool
	changed      chan struct{}
	done         chan struct{}
	doneOnce     sync.Once
	continuation *fetchContinuation
}

func newAutomaticJobState(force bool) *automaticJobState {
	return &automaticJobState{
		force:      force,
		generation: 1,
		changed:    make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func (s *automaticJobState) currentGeneration() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

func (s *automaticJobState) forceRequested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.force
}

func (s *automaticJobState) isCancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled
}

func (s *automaticJobState) admissionWaitSnapshot(expectedGeneration uint64) (<-chan struct{}, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled {
		return nil, false, errAutomaticJobInvalidated
	}
	if s.generation != expectedGeneration {
		return nil, true, nil
	}
	return s.changed, false, nil
}

func (s *automaticJobState) getContinuation() *fetchContinuation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.continuation
}

func (s *automaticJobState) setContinuation(continuation *fetchContinuation) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled {
		return false
	}
	s.continuation = continuation
	return true
}

func (s *automaticJobState) restartContinuation() {
	s.mu.Lock()
	s.generation++
	s.continuation = nil
	s.signalChangedLocked()
	s.mu.Unlock()
}

func (s *automaticJobState) requestForceAndRestart() {
	s.mu.Lock()
	s.force = true
	s.generation++
	s.continuation = nil
	s.signalChangedLocked()
	s.mu.Unlock()
}

func (s *automaticJobState) invalidate(cancel bool) {
	s.mu.Lock()
	s.generation++
	s.continuation = nil
	if cancel {
		s.cancelled = true
	}
	s.signalChangedLocked()
	s.mu.Unlock()
}

func (s *automaticJobState) signalChangedLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *automaticJobState) finish() {
	s.mu.Lock()
	s.cancelled = true
	s.generation++
	s.continuation = nil
	s.signalChangedLocked()
	s.doneOnce.Do(func() { close(s.done) })
	s.mu.Unlock()
}

type queuedAutomaticJob struct {
	job     automaticJob
	discard func()
}

type automaticJobResult struct {
	wait    func(context.Context) error
	discard func()
}

// automaticScheduler owns the bounded execution pool. The queue may contain
// one entry per configured workspace, but only workers execute provider calls.
// A condition variable avoids a feeder goroutine and makes cancellation able
// to discard queued jobs synchronously.
type automaticScheduler struct {
	mu       sync.Mutex
	cond     *sync.Cond
	queue    []queuedAutomaticJob
	closing  bool
	wg       sync.WaitGroup
	watchers sync.WaitGroup
}

func newAutomaticScheduler(ctx context.Context, workers int, run func(context.Context, automaticJob) automaticJobResult, idle func()) *automaticScheduler {
	if workers < 1 {
		workers = 1
	}
	s := &automaticScheduler{}
	s.cond = sync.NewCond(&s.mu)
	s.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go s.worker(ctx, run, idle)
	}
	return s
}

func (s *automaticScheduler) enqueue(job automaticJob) bool {
	return s.enqueueOwned(queuedAutomaticJob{job: job})
}

func (s *automaticScheduler) enqueueOwned(job queuedAutomaticJob) bool {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		if job.discard != nil {
			job.discard()
		}
		return false
	}
	s.queue = append(s.queue, job)
	s.cond.Signal()
	s.mu.Unlock()
	return true
}

func (s *automaticScheduler) worker(ctx context.Context, run func(context.Context, automaticJob) automaticJobResult, idle func()) {
	defer s.wg.Done()
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closing {
			s.cond.Wait()
		}
		if s.closing || ctx.Err() != nil {
			discard := s.takeQueuedDiscardsLocked()
			s.mu.Unlock()
			for _, fn := range discard {
				fn()
			}
			return
		}
		queued := s.queue[0]
		copy(s.queue, s.queue[1:])
		s.queue = s.queue[:len(s.queue)-1]
		s.mu.Unlock()
		result := run(ctx, queued.job)
		if result.wait == nil {
			idle()
			continue
		}
		s.watchers.Add(1)
		go func() {
			defer s.watchers.Done()
			if err := result.wait(ctx); err != nil {
				if result.discard != nil {
					result.discard()
				}
				return
			}
			s.enqueueOwned(queuedAutomaticJob{job: queued.job, discard: result.discard})
		}()
	}
}

func (s *automaticScheduler) takeQueuedDiscardsLocked() []func() {
	discard := make([]func(), 0, len(s.queue))
	for _, queued := range s.queue {
		if queued.discard != nil {
			discard = append(discard, queued.discard)
		}
	}
	s.queue = nil
	return discard
}

func (s *automaticScheduler) close() {
	s.mu.Lock()
	s.closing = true
	discard := s.takeQueuedDiscardsLocked()
	s.cond.Broadcast()
	s.mu.Unlock()
	for _, fn := range discard {
		fn()
	}
}

func (s *automaticScheduler) stop() {
	s.close()
	s.wg.Wait()
	s.watchers.Wait()
}
