package recorder

import (
	"context"
	"sync"

	"github.com/kandev/kandev/internal/coordinator"
)

const captureQueueCap = 1000

type captureJob struct {
	ctx context.Context
	ev  coordinator.DecisionEvent
}

type decisionCaptureQueue struct {
	owner  *Observers
	target coordinator.DecisionObserver
	jobs   chan captureJob
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func newDecisionCaptureQueue(owner *Observers, target coordinator.DecisionObserver) *decisionCaptureQueue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &decisionCaptureQueue{owner: owner, target: target, jobs: make(chan captureJob, captureQueueCap), ctx: ctx, cancel: cancel}
	q.wg.Add(1)
	go q.run()
	return q
}

func (q *decisionCaptureQueue) OnDecision(ctx context.Context, ev coordinator.DecisionEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		bump(captureDroppedTotal, CaptureDroppedStopped)
		return
	}
	ev.EditedFields = append([]string(nil), ev.EditedFields...)
	select {
	case q.jobs <- captureJob{ctx: context.WithoutCancel(ctx), ev: ev}:
	default:
		bump(captureDroppedTotal, CaptureDroppedQueueFull)
	}
}

func (q *decisionCaptureQueue) Stop() {
	q.mu.Lock()
	q.closed = true
	q.cancel()
	q.mu.Unlock()
	q.wg.Wait()
}

func (q *decisionCaptureQueue) run() {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			q.discardPending()
			return
		case job := <-q.jobs:
			ctx, cancel := context.WithCancel(job.ctx)
			stopCancel := context.AfterFunc(q.ctx, cancel)
			q.owner.call(ctx, namedObserver{ObserverOverride, q.target}, job.ev)
			stopCancel()
			cancel()
		}
	}
}

func (q *decisionCaptureQueue) discardPending() {
	for {
		select {
		case <-q.jobs:
			bump(captureDroppedTotal, CaptureDroppedStopped)
		default:
			return
		}
	}
}
