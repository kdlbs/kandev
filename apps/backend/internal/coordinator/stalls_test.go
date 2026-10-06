package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func newTestServiceForSubscribers(t *testing.T) (*Service, *Store) {
	t.Helper()
	store := newTestStore(t)
	svc := NewService(store, NewValidator(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	return svc, store
}

// captureCoordinatorUpdated collects every coordinator.updated event
// published on a bus, in publish order. MemoryEventBus dispatches
// synchronously, so a test can read capture after Publish returns.
type captureCoordinatorUpdated struct {
	mu     sync.Mutex
	events []CoordinatorUpdatedPayload
}

func (c *captureCoordinatorUpdated) record(_ context.Context, e *bus.Event) error {
	payload, ok := e.Data.(CoordinatorUpdatedPayload)
	if !ok {
		return nil
	}
	c.mu.Lock()
	c.events = append(c.events, payload)
	c.mu.Unlock()
	return nil
}

func (c *captureCoordinatorUpdated) snapshot() []CoordinatorUpdatedPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CoordinatorUpdatedPayload, len(c.events))
	copy(out, c.events)
	return out
}

// selectiveFailBus wraps a bus.EventBus, failing Publish for events accepted
// by shouldFail without forwarding them, while every other event and every
// other method delegates to inner. Models a transient publish failure for
// one coordinator's coordinator.updated event.
type selectiveFailBus struct {
	inner      bus.EventBus
	shouldFail func(event *bus.Event) bool
}

func (b *selectiveFailBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if b.shouldFail(event) {
		return errPublishFailed
	}
	return b.inner.Publish(ctx, subject, event)
}

func (b *selectiveFailBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return b.inner.Subscribe(subject, handler)
}

func (b *selectiveFailBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return b.inner.QueueSubscribe(subject, queue, handler)
}

func (b *selectiveFailBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return b.inner.Request(ctx, subject, event, timeout)
}

func (b *selectiveFailBus) Close()            { b.inner.Close() }
func (b *selectiveFailBus) IsConnected() bool { return b.inner.IsConnected() }

var errPublishFailed = errors.New("simulated publish failure")

func taskStalledEvent(taskID, workspaceID string, stalledFor time.Duration, lastEventAt time.Time) *bus.Event {
	return bus.NewEvent(events.TaskStalled, "task-service", map[string]interface{}{
		"task_id":        taskID,
		"workspace_id":   workspaceID,
		"stalled_for":    stalledFor.String(),
		"last_event_at":  lastEventAt.UTC().Format(time.RFC3339Nano),
		"detection_only": true,
	})
}

func TestSubscribeTaskStalled_UpsertsAndPublishesPerCoordinatorInListOrder(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	memBus := bus.NewMemoryEventBus(newTestLogger(t))

	first := newTestCoordinator(t, store, "ws-1")
	second := newTestCoordinator(t, store, "ws-1")
	// Give the second coordinator one open (pending) proposal so the
	// captured payloads are distinguishable by open_proposals too.
	p := &Proposal{CoordinatorID: second.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
	if err := store.InsertProposal(ctx, p); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	capture := &captureCoordinatorUpdated{}
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, capture.record); err != nil {
		t.Fatalf("subscribe coordinator.updated: %v", err)
	}

	if _, err := SubscribeTaskStalled(memBus, svc, newTestLogger(t)); err != nil {
		t.Fatalf("SubscribeTaskStalled: %v", err)
	}

	lastEventAt := time.Now().UTC()
	evt := taskStalledEvent("task-1", "ws-1", 90*time.Second, lastEventAt)
	if err := memBus.Publish(ctx, events.TaskStalled, evt); err != nil {
		t.Fatalf("Publish task.stalled: %v", err)
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 1 || list[0].TaskID != "task-1" || list[0].StalledForMs != 90_000 {
		t.Fatalf("ListStalls = %+v, want one row for task-1 at 90000ms", list)
	}

	got := capture.snapshot()
	if len(got) != 2 {
		t.Fatalf("coordinator.updated captured %d events, want 2", len(got))
	}
	if got[0].CoordinatorID != first.ID || got[0].OpenProposals != 0 {
		t.Fatalf("coordinator.updated[0] = %+v, want coordinator %s with 0 open proposals", got[0], first.ID)
	}
	if got[1].CoordinatorID != second.ID || got[1].OpenProposals != 1 {
		t.Fatalf("coordinator.updated[1] = %+v, want coordinator %s with 1 open proposal", got[1], second.ID)
	}
	for _, payload := range got {
		if payload.WorkspaceID != "ws-1" {
			t.Fatalf("coordinator.updated payload workspace_id = %q, want ws-1", payload.WorkspaceID)
		}
	}
}

func TestSubscribeTaskStalled_NoCoordinatorsSkipsUpsertAndPublish(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	memBus := bus.NewMemoryEventBus(newTestLogger(t))

	capture := &captureCoordinatorUpdated{}
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, capture.record); err != nil {
		t.Fatalf("subscribe coordinator.updated: %v", err)
	}
	if _, err := SubscribeTaskStalled(memBus, svc, newTestLogger(t)); err != nil {
		t.Fatalf("SubscribeTaskStalled: %v", err)
	}

	evt := taskStalledEvent("task-1", "ws-uncoordinated", time.Minute, time.Now().UTC())
	if err := memBus.Publish(ctx, events.TaskStalled, evt); err != nil {
		t.Fatalf("Publish task.stalled: %v", err)
	}

	list, err := store.ListStalls(ctx, "ws-uncoordinated")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListStalls = %+v, want no rows for an uncoordinated workspace", list)
	}
	if len(capture.snapshot()) != 0 {
		t.Fatal("coordinator.updated published for an uncoordinated workspace")
	}
}

func TestSubscribeTaskStalled_OutOfOrderEventSkipsPublish(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	newTestCoordinator(t, store, "ws-1")

	latest := time.Now().UTC()
	if _, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", latest)); err != nil {
		t.Fatalf("seed UpsertStall: %v", err)
	}

	capture := &captureCoordinatorUpdated{}
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, capture.record); err != nil {
		t.Fatalf("subscribe coordinator.updated: %v", err)
	}
	if _, err := SubscribeTaskStalled(memBus, svc, newTestLogger(t)); err != nil {
		t.Fatalf("SubscribeTaskStalled: %v", err)
	}

	earlier := taskStalledEvent("task-1", "ws-1", time.Minute, latest.Add(-time.Minute))
	if err := memBus.Publish(ctx, events.TaskStalled, earlier); err != nil {
		t.Fatalf("Publish task.stalled: %v", err)
	}

	if len(capture.snapshot()) != 0 {
		t.Fatal("coordinator.updated published for an out-of-order stall event")
	}
}

func TestSubscribeTaskStalled_MalformedEventIsDroppedWithoutError(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	newTestCoordinator(t, store, "ws-1")

	if _, err := SubscribeTaskStalled(memBus, svc, newTestLogger(t)); err != nil {
		t.Fatalf("SubscribeTaskStalled: %v", err)
	}

	malformed := bus.NewEvent(events.TaskStalled, "task-service", map[string]interface{}{
		"task_id": "task-1",
		// workspace_id, stalled_for, last_event_at are missing.
	})
	if err := memBus.Publish(ctx, events.TaskStalled, malformed); err != nil {
		t.Fatalf("Publish malformed task.stalled: %v", err)
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListStalls = %+v, want no rows from a malformed event", list)
	}
}

func TestSubscribeTaskStalled_PublishFailureForOneCoordinatorSkipsOnlyThatCoordinator(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	inner := bus.NewMemoryEventBus(newTestLogger(t))

	first := newTestCoordinator(t, store, "ws-1")
	second := newTestCoordinator(t, store, "ws-1")

	capture := &captureCoordinatorUpdated{}
	if _, err := inner.Subscribe(events.CoordinatorUpdated, capture.record); err != nil {
		t.Fatalf("subscribe coordinator.updated: %v", err)
	}

	failingBus := &selectiveFailBus{
		inner: inner,
		shouldFail: func(event *bus.Event) bool {
			payload, ok := event.Data.(CoordinatorUpdatedPayload)
			return ok && payload.CoordinatorID == first.ID
		},
	}

	if _, err := SubscribeTaskStalled(failingBus, svc, newTestLogger(t)); err != nil {
		t.Fatalf("SubscribeTaskStalled: %v", err)
	}

	evt := taskStalledEvent("task-1", "ws-1", time.Minute, time.Now().UTC())
	if err := failingBus.Publish(ctx, events.TaskStalled, evt); err != nil {
		t.Fatalf("Publish task.stalled: %v", err)
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 1 || list[0].TaskID != "task-1" {
		t.Fatalf("ListStalls = %+v, want the stall row written despite the publish failure", list)
	}

	got := capture.snapshot()
	if len(got) != 1 || got[0].CoordinatorID != second.ID {
		t.Fatalf("coordinator.updated captured = %+v, want exactly one event for coordinator %s", got, second.ID)
	}
}
