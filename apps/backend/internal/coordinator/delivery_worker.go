package coordinator

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// deliveryWorker runs Deliver for the coordinators that were kicked. At most
// one goroutine runs per coordinator; a kick that arrives during a run makes
// that goroutine run once more when it finishes.
type deliveryWorker struct {
	mu      sync.Mutex
	svc     *Service
	run     func(ctx context.Context, coordinatorID string) error
	ctx     context.Context
	cancel  context.CancelFunc
	active  map[string]*deliveryRun
	subs    []bus.Subscription
	started bool
	closed  bool
	wg      sync.WaitGroup
}

type deliveryRun struct{ again bool }

// StartDelivery starts the worker, makes Kick the service's kick and
// subscribes to turn end and conversation-idle events. eventBus may be nil.
// It is a no-op after StopDelivery.
func (s *Service) StartDelivery(eventBus bus.EventBus) {
	w := &s.delivery
	w.mu.Lock()
	if w.started || w.closed {
		w.mu.Unlock()
		return
	}
	w.svc = s
	if w.run == nil {
		w.run = s.Deliver
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.active = map[string]*deliveryRun{}
	w.started = true
	w.mu.Unlock()
	s.SetKick(w.Kick)
	s.SetBackstopHooks(Hooks{TurnDuties: s.turnDuties, Deliver: w.Kick})
	if eventBus == nil {
		return
	}
	subs := []struct {
		subject string
		handle  func(context.Context, *bus.Event)
	}{
		{events.TurnCompleted, s.onTurnCompleted},
		{events.TaskSessionStateChanged, s.onSessionStateChanged},
	}
	for _, sub := range subs {
		subscription, err := eventBus.Subscribe(sub.subject, w.handler(sub.handle))
		if err != nil {
			s.logger.Warn("delivery subscription failed", zap.String("subject", sub.subject), zap.Error(err))
			continue
		}
		w.keep(subscription)
	}
}

func (w *deliveryWorker) keep(sub bus.Subscription) {
	w.mu.Lock()
	closed := w.closed
	if !closed {
		w.subs = append(w.subs, sub)
	}
	w.mu.Unlock()
	if closed {
		_ = sub.Unsubscribe()
	}
}

// enter registers one unit of work in flight; ok is false once stopped.
func (w *deliveryWorker) enter() (context.Context, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.started || w.closed {
		return nil, false
	}
	w.wg.Add(1)
	return w.ctx, true
}

func (w *deliveryWorker) handler(fn func(context.Context, *bus.Event)) bus.EventHandler {
	return func(_ context.Context, event *bus.Event) error {
		ctx, ok := w.enter()
		if !ok {
			return nil
		}
		defer w.wg.Done()
		defer w.svc.recoverWake("delivery event handler")
		fn(ctx, event)
		return nil
	}
}

// Kick asks for one delivery pass for the coordinator and returns at once.
func (w *deliveryWorker) Kick(_ context.Context, coordinatorID string) error {
	ctx, ok := w.enter()
	if !ok {
		return nil
	}
	w.mu.Lock()
	if r, running := w.active[coordinatorID]; running {
		r.again = true
		w.mu.Unlock()
		w.wg.Done()
		return nil
	}
	w.active[coordinatorID] = &deliveryRun{}
	w.mu.Unlock()
	go w.loop(ctx, coordinatorID)
	return nil
}

func (w *deliveryWorker) loop(ctx context.Context, coordinatorID string) {
	defer w.wg.Done()
	for {
		w.runOnce(ctx, coordinatorID)
		w.mu.Lock()
		r := w.active[coordinatorID]
		if !r.again || ctx.Err() != nil {
			delete(w.active, coordinatorID)
			w.mu.Unlock()
			return
		}
		r.again = false
		w.mu.Unlock()
	}
}

func (w *deliveryWorker) runOnce(ctx context.Context, coordinatorID string) {
	defer w.svc.recoverWake("delivery run")
	if err := w.run(ctx, coordinatorID); err != nil && ctx.Err() == nil {
		w.svc.logger.Warn("coordinator delivery failed", zap.String("coordinator_id", coordinatorID), zap.Error(err))
	}
}

// StopDelivery unsubscribes, cancels the worker and waits for the work in
// flight. It is idempotent and latches the worker closed.
func (s *Service) StopDelivery() {
	w := &s.delivery
	w.mu.Lock()
	w.closed = true
	cancel := w.cancel
	subs := w.subs
	w.subs = nil
	w.mu.Unlock()
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	if cancel != nil {
		cancel()
	}
	w.wg.Wait()
}

// onSessionStateChanged kicks the coordinator whose conversation session just
// became idle.
func (s *Service) onSessionStateChanged(ctx context.Context, event *bus.Event) {
	data, ok := eventData(event)
	if !ok || eventString(data, "new_state") != string(taskmodels.TaskSessionStateWaitingForInput) {
		return
	}
	taskID, sessionID := eventString(data, "task_id"), eventString(data, "session_id")
	if taskID == "" || sessionID == "" || s.conversationTasks == nil || s.convReader == nil {
		return
	}
	task, err := s.conversationTasks.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	coordinatorID := conversationTaskCoordinatorID(task)
	if coordinatorID == "" || isDreamTask(task) {
		return
	}
	primary, err := s.convReader.PrimarySession(ctx, taskID)
	if err != nil || primary == nil || primary.ID != sessionID {
		return
	}
	s.callKick(ctx, coordinatorID)
}
