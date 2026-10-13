package coordinator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type runRecorder struct {
	mu      sync.Mutex
	runs    map[string]int
	started chan string
	release chan struct{}
}

func newRunRecorder() *runRecorder {
	return &runRecorder{runs: map[string]int{}, started: make(chan string, 16), release: make(chan struct{}, 16)}
}

func (r *runRecorder) run(_ context.Context, id string) error {
	r.mu.Lock()
	r.runs[id]++
	r.mu.Unlock()
	r.started <- id
	<-r.release
	return nil
}

func (r *runRecorder) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs[id]
}

func waitStarted(t *testing.T, r *runRecorder) string {
	t.Helper()
	select {
	case id := <-r.started:
		return id
	case <-time.After(5 * time.Second):
		t.Fatal("no delivery run started")
		return ""
	}
}

func workerFixture(t *testing.T, r *runRecorder) *Service {
	t.Helper()
	svc := newWakeEnv(t).svc
	svc.delivery.run = r.run
	svc.StartDelivery(nil)
	t.Cleanup(svc.StopDelivery)
	return svc
}

func TestKick_RunsDeliveryForTheCoordinator(t *testing.T) {
	r := newRunRecorder()
	svc := workerFixture(t, r)
	if err := svc.delivery.Kick(context.Background(), "co-1"); err != nil {
		t.Fatal(err)
	}
	if got := waitStarted(t, r); got != "co-1" {
		t.Fatalf("started %q", got)
	}
	r.release <- struct{}{}
}

func TestKick_KicksDuringARunCoalesceIntoOneRerun(t *testing.T) {
	r := newRunRecorder()
	svc := workerFixture(t, r)
	_ = svc.delivery.Kick(context.Background(), "co-1")
	waitStarted(t, r)
	for i := 0; i < 5; i++ {
		_ = svc.delivery.Kick(context.Background(), "co-1")
	}
	r.release <- struct{}{}
	waitStarted(t, r)
	r.release <- struct{}{}
	svc.StopDelivery()
	if got := r.count("co-1"); got != 2 {
		t.Fatalf("runs = %d, want 2", got)
	}
}

func TestKick_DifferentCoordinatorsRunConcurrently(t *testing.T) {
	r := newRunRecorder()
	svc := workerFixture(t, r)
	_ = svc.delivery.Kick(context.Background(), "co-1")
	_ = svc.delivery.Kick(context.Background(), "co-2")
	got := map[string]bool{waitStarted(t, r): true, waitStarted(t, r): true}
	if !got["co-1"] || !got["co-2"] {
		t.Fatalf("started %v", got)
	}
	r.release <- struct{}{}
	r.release <- struct{}{}
}

func TestStopDelivery_WaitsForARunInFlightAndLatchesKicksOff(t *testing.T) {
	r := newRunRecorder()
	svc := workerFixture(t, r)
	_ = svc.delivery.Kick(context.Background(), "co-1")
	waitStarted(t, r)
	var stopped atomic.Bool
	done := make(chan struct{})
	go func() {
		svc.StopDelivery()
		stopped.Store(true)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	if stopped.Load() {
		t.Fatal("StopDelivery returned before the run finished")
	}
	r.release <- struct{}{}
	<-done
	_ = svc.delivery.Kick(context.Background(), "co-1")
	svc.StopDelivery()
	if got := r.count("co-1"); got != 1 {
		t.Fatalf("runs = %d, want 1", got)
	}
}

func TestKick_BeforeStartIsANoOp(t *testing.T) {
	r := newRunRecorder()
	svc := newWakeEnv(t).svc
	svc.delivery.run = r.run
	if err := svc.delivery.Kick(context.Background(), "co-1"); err != nil {
		t.Fatal(err)
	}
	if r.count("co-1") != 0 {
		t.Fatal("kick before start ran delivery")
	}
}

func stateEvent(taskID, sessionID, state string) *bus.Event {
	return &bus.Event{Type: events.TaskSessionStateChanged, Data: map[string]any{
		"task_id": taskID, "session_id": sessionID, "new_state": state,
	}}
}

func TestOnSessionStateChanged_WaitingKicksTheConversationsCoordinator(t *testing.T) {
	f := newDeliverFixture(t)
	var kicked []string
	f.svc.SetKick(func(_ context.Context, id string) error { kicked = append(kicked, id); return nil })
	f.svc.onSessionStateChanged(context.Background(), stateEvent(ceilingConvTask, ceilingSession, string(taskmodels.TaskSessionStateWaitingForInput)))
	if len(kicked) != 1 || kicked[0] != f.c.ID {
		t.Fatalf("kicked = %v", kicked)
	}
}

func TestOnSessionStateChanged_IgnoresOtherStatesTasksAndSessions(t *testing.T) {
	f := newDeliverFixture(t)
	f.conv.tasks["plain"] = &taskmodels.Task{ID: "plain"}
	var kicked []string
	f.svc.SetKick(func(_ context.Context, id string) error { kicked = append(kicked, id); return nil })
	ctx := context.Background()
	waiting := string(taskmodels.TaskSessionStateWaitingForInput)
	f.svc.onSessionStateChanged(ctx, stateEvent(ceilingConvTask, ceilingSession, string(taskmodels.TaskSessionStateRunning)))
	f.svc.onSessionStateChanged(ctx, stateEvent("plain", ceilingSession, waiting))
	f.svc.onSessionStateChanged(ctx, stateEvent("missing", ceilingSession, waiting))
	f.svc.onSessionStateChanged(ctx, stateEvent(ceilingConvTask, "other-session", waiting))
	f.svc.onSessionStateChanged(ctx, &bus.Event{Data: "not a map"})
	if len(kicked) != 0 {
		t.Fatalf("kicked = %v, want none", kicked)
	}
}

func TestStartDelivery_SubscribesToTurnEndAndConversationIdle(t *testing.T) {
	f := newDeliverFixture(t)
	r := newRunRecorder()
	f.svc.delivery.run = r.run
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	f.svc.StartDelivery(eventBus)
	t.Cleanup(f.svc.StopDelivery)
	if err := eventBus.Publish(context.Background(), events.TaskSessionStateChanged,
		stateEvent(ceilingConvTask, ceilingSession, string(taskmodels.TaskSessionStateWaitingForInput))); err != nil {
		t.Fatal(err)
	}
	if got := waitStarted(t, r); got != f.c.ID {
		t.Fatalf("started %q", got)
	}
	r.release <- struct{}{}
}

func TestStartDelivery_InstallsTheBackstopDuties(t *testing.T) {
	svc := newWakeEnv(t).svc
	svc.StartDelivery(nil)
	t.Cleanup(svc.StopDelivery)
	hooks := svc.backstop.currentHooks()
	if hooks.TurnDuties == nil || hooks.Deliver == nil {
		t.Fatalf("hooks = %+v, want TurnDuties and Deliver set", hooks)
	}
}
