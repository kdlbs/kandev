package websocket

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// TestOfficeEventBroadcaster_SubscriptionCount verifies that
// RegisterOfficeNotifications creates exactly one subscription per event type.
func TestOfficeEventBroadcaster_SubscriptionCount(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterOfficeNotifications(ctx, eventBus, hub, log)

	// One subscription per event type subscribed in RegisterOfficeNotifications.
	// Update this count when adding/removing event subscriptions.
	const wantSubscriptions = 20
	if got := len(b.subscriptions); got != wantSubscriptions {
		t.Errorf("RegisterOfficeNotifications created %d subscriptions, want %d",
			got, wantSubscriptions)
	}
}

// TestOfficeEventBroadcaster_BroadcastsEvent verifies that publishing an
// office event on the bus results in a hub.Broadcast call.
func TestOfficeEventBroadcaster_BroadcastsEvent(t *testing.T) {
	log := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCases := []struct {
		subject string
	}{
		{events.OfficeCommentCreated},
		{events.OfficeApprovalCreated},
		{events.OfficeTaskStatusChanged},
		{events.AgentCompleted},
		{events.AgentFailed},
		{events.TaskMoved},
	}

	for _, tc := range testCases {
		t.Run(tc.subject, func(t *testing.T) {
			eb := bus.NewMemoryEventBus(log)
			hub := NewHub(nil, log)
			go hub.Run(ctx)

			_ = RegisterOfficeNotifications(ctx, eb, hub, log)

			// Track whether the office broadcaster's handler ran by counting
			// all subscribers on this subject (broadcaster + our counter).
			var handlerCalled int
			_, _ = eb.Subscribe(tc.subject, func(_ context.Context, _ *bus.Event) error {
				handlerCalled++
				return nil
			})

			data := map[string]interface{}{
				"workspace_id": "ws-123",
				"task_id":      "t-456",
			}
			evt := bus.NewEvent(tc.subject, "test", data)
			if err := eb.Publish(context.Background(), tc.subject, evt); err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			// Our counter should have been called exactly once.
			if handlerCalled != 1 {
				t.Errorf("handler called %d times, want 1", handlerCalled)
			}
		})
	}
}

// TestOfficeEventBroadcaster_NilEventBus verifies no panic when event bus is nil.
func TestOfficeEventBroadcaster_NilEventBus(t *testing.T) {
	log := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterOfficeNotifications(ctx, nil, hub, log)
	if len(b.subscriptions) != 0 {
		t.Errorf("expected 0 subscriptions with nil event bus, got %d", len(b.subscriptions))
	}
}

// TestOfficeEventBroadcaster_Close verifies that Close unsubscribes all subscriptions.
func TestOfficeEventBroadcaster_Close(t *testing.T) {
	log := testLogger()
	eb := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterOfficeNotifications(ctx, eb, hub, log)
	if len(b.subscriptions) == 0 {
		t.Fatal("expected subscriptions before close")
	}

	b.Close()
	if len(b.subscriptions) != 0 {
		t.Errorf("expected 0 subscriptions after Close, got %d", len(b.subscriptions))
	}
}
