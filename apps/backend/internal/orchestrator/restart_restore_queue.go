package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"sync"
)

const restartRestoreConcurrency = 2

type restartRestoreJob struct {
	candidate models.SilentRestoreCandidate
	run       func(context.Context, models.SilentRestoreCandidate) error
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	err       error
}

type restartRestoreQueue struct {
	mu      sync.Mutex
	ctx     context.Context
	jobs    map[string]*restartRestoreJob
	pending []*restartRestoreJob
	focused string
	active  int
	stopped bool
	workers sync.WaitGroup
}

func newRestartRestoreQueue(ctx context.Context) *restartRestoreQueue {
	return &restartRestoreQueue{ctx: ctx, jobs: make(map[string]*restartRestoreJob)}
}

func (q *restartRestoreQueue) submit(candidate models.SilentRestoreCandidate, run func(context.Context, models.SilentRestoreCandidate) error) *restartRestoreJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job := q.jobs[candidate.SessionID]; job != nil {
		return job
	}
	ctx, cancel := context.WithCancel(q.ctx)
	job := &restartRestoreJob{candidate: candidate, run: run, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	if q.stopped || q.ctx.Err() != nil {
		cancel()
		job.err = context.Canceled
		close(job.done)
		return job
	}
	q.jobs[candidate.SessionID] = job
	q.pending = append(q.pending, job)
	q.dispatchLocked()
	return job
}

func (q *restartRestoreQueue) prioritize(sessionID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.focused = sessionID
}

func (q *restartRestoreQueue) dispatchLocked() {
	for !q.stopped && q.active < restartRestoreConcurrency && len(q.pending) > 0 {
		index := 0
		for i, job := range q.pending {
			if job.candidate.SessionID == q.focused {
				index = i
				break
			}
		}
		job := q.pending[index]
		q.pending = append(q.pending[:index], q.pending[index+1:]...)
		q.active++
		q.workers.Add(1)
		go q.run(job)
	}
}

func (q *restartRestoreQueue) run(job *restartRestoreJob) {
	defer q.workers.Done()
	ctx, cancel := context.WithTimeout(job.ctx, silentRestoreOperationLimit)
	err := job.run(ctx, job.candidate)
	cancel()
	job.cancel()
	q.mu.Lock()
	defer q.mu.Unlock()
	job.err = err
	delete(q.jobs, job.candidate.SessionID)
	q.active--
	close(job.done)
	q.dispatchLocked()
}

func (q *restartRestoreQueue) stop() {
	q.mu.Lock()
	q.stopped = true
	for _, job := range q.jobs {
		job.cancel()
	}
	for _, job := range q.pending {
		job.err = context.Canceled
		close(job.done)
		delete(q.jobs, job.candidate.SessionID)
	}
	q.pending = nil
	q.mu.Unlock()
	q.workers.Wait()
}

func (s *Service) restartRestoreScheduler(ctx context.Context) *restartRestoreQueue {
	s.silentRestoreMu.Lock()
	defer s.silentRestoreMu.Unlock()
	if s.silentRestoreQueue == nil {
		s.silentRestoreQueue = newRestartRestoreQueue(ctx)
	}
	return s.silentRestoreQueue
}
