package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"sync"
)

const restartRestoreConcurrency = 2

type restartRestoreAction uint8

const (
	restartRestoreNative restartRestoreAction = iota
	restartRestoreAutomatic
)

type restartRestoreJob struct {
	action    restartRestoreAction
	candidate models.SilentRestoreCandidate
	run       func(context.Context, models.SilentRestoreCandidate) error
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	err       error
}

type restartRestoreWork struct {
	job       *restartRestoreJob
	action    restartRestoreAction
	candidate models.SilentRestoreCandidate
	run       func(context.Context, models.SilentRestoreCandidate) error
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
	return q.submitAction(candidate, restartRestoreNative, run)
}

func (q *restartRestoreQueue) submitAction(candidate models.SilentRestoreCandidate, action restartRestoreAction, run func(context.Context, models.SilentRestoreCandidate) error) *restartRestoreJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job := q.jobs[candidate.SessionID]; job != nil {
		return job
	}
	ctx, cancel := context.WithCancel(q.ctx)
	job := &restartRestoreJob{action: action, candidate: candidate, run: run, ctx: ctx, cancel: cancel, done: make(chan struct{})}
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

func (q *restartRestoreQueue) submitWork(candidate models.SilentRestoreCandidate, action restartRestoreAction, run func(context.Context, models.SilentRestoreCandidate) error) *restartRestoreWork {
	return &restartRestoreWork{job: q.submitAction(candidate, action, run), action: action, candidate: candidate, run: run}
}

// Matching actions share completion; a different action waits for its own session job.
func (q *restartRestoreQueue) waitForWork(ctx context.Context, work *restartRestoreWork) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-work.job.done:
		}
		if work.job.action == work.action {
			return true
		}
		q.mu.Lock()
		stopped := q.stopped || q.ctx.Err() != nil
		q.mu.Unlock()
		if stopped {
			return false
		}
		work = q.submitWork(work.candidate, work.action, work.run)
	}
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
