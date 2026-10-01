package recorder

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	gradeQueueCap  = 1000
	sweepInterval  = 24 * time.Hour
	sweepBatchSize = 200
)

// Queue grades proposals on one worker. Each proposal is queued at most once;
// a full queue drops the request and counts it, the sweep re-finds it later.
type Queue struct {
	grader *Grader
	log    *zap.Logger

	mu      sync.Mutex
	pending map[string]time.Time
	order   chan string
	stopCh  chan struct{}
	stopped bool
	wg      sync.WaitGroup
}

// NewQueue builds a stopped queue over grader.
func NewQueue(grader *Grader, log *zap.Logger) *Queue {
	if log == nil {
		log = zap.NewNop()
	}
	return &Queue{grader: grader, log: log, pending: map[string]time.Time{}, order: make(chan string, gradeQueueCap), stopCh: make(chan struct{})}
}

// Enqueue asks for proposalID to be graded; decidedAt is the hook's decision
// time or zero.
func (q *Queue) Enqueue(proposalID string, decidedAt time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return
	}
	if at, queued := q.pending[proposalID]; queued {
		if at.IsZero() {
			q.pending[proposalID] = decidedAt
		}
		return
	}
	select {
	case q.order <- proposalID:
		q.pending[proposalID] = decidedAt
	default:
		bump(gradeFailedTotal, GradeQueueFull)
	}
}

// Start runs the worker.
func (q *Queue) Start(ctx context.Context) {
	q.wg.Add(1)
	go q.run(ctx)
}

// Stop ends the worker and waits for it; it is idempotent.
func (q *Queue) Stop() {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.stopped = true
	close(q.stopCh)
	q.mu.Unlock()
	q.wg.Wait()
}

func (q *Queue) run(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case <-q.stopCh:
			return
		case <-ctx.Done():
			return
		case id := <-q.order:
			q.mu.Lock()
			at := q.pending[id]
			delete(q.pending, id)
			q.mu.Unlock()
			q.gradeOne(ctx, id, at)
		}
	}
}

func (q *Queue) gradeOne(ctx context.Context, id string, at time.Time) {
	defer func() {
		if r := recover(); r != nil {
			bump(observerPanicTotal, ObserverGrader)
			q.log.Error("coordinator outcome grader: panic", zap.String("proposal_id", id), zap.Any("panic", r))
		}
	}()
	if err := q.grader.Grade(ctx, id, at); err != nil {
		q.log.Warn("coordinator outcome grade failed", zap.String("proposal_id", id), zap.Error(err))
	}
}

// OnDecision implements coordinator.DecisionObserver: it queues the decided
// proposal and never blocks or fails the decision.
func (q *Queue) OnDecision(_ context.Context, ev coordinator.DecisionEvent) {
	q.Enqueue(ev.ProposalID, ev.At)
}

// Subscribe queues the open-outcome proposals of a task whenever it moves or
// changes state. The returned function unsubscribes.
func (q *Queue) Subscribe(eventBus bus.EventBus, db taskProposals) (func(), error) {
	var subs []bus.Subscription
	for _, subject := range []string{events.TaskMoved, events.TaskStateChanged} {
		sub, err := eventBus.Subscribe(subject, func(ctx context.Context, e *bus.Event) error {
			q.onTaskEvent(ctx, db, e)
			return nil
		})
		if err != nil {
			for _, done := range subs {
				_ = done.Unsubscribe()
			}
			return nil, err
		}
		subs = append(subs, sub)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for _, sub := range subs {
				_ = sub.Unsubscribe()
			}
		})
	}, nil
}

// taskProposals lists the decided proposals of a task still being graded.
type taskProposals interface {
	OpenProposalsOfTask(ctx context.Context, taskID string) ([]string, error)
}

func (q *Queue) onTaskEvent(ctx context.Context, db taskProposals, e *bus.Event) {
	data, _ := e.Data.(map[string]any)
	taskID, _ := data["task_id"].(string)
	if taskID == "" {
		return
	}
	ids, err := db.OpenProposalsOfTask(ctx, taskID)
	if err != nil {
		bump(gradeFailedTotal, GradeProposalRead)
		return
	}
	for _, id := range ids {
		q.Enqueue(id, time.Time{})
	}
}
