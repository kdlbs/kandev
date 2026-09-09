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
