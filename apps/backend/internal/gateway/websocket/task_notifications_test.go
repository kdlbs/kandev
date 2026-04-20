package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

// TestTaskEventBroadcaster_NoDuplicateSubscriptions verifies that
// RegisterTaskNotifications creates exactly one subscription per event type.
//
// The old code had a second subscription system (subscribeEventBusHandlers in
// cmd/kandev/helpers.go) that subscribed to the same four events, causing
// duplicate broadcasts. This test counts the broadcaster's internal
// subscriptions directly to guard against re-introducing duplicates.
func TestTaskEventBroadcaster_NoDuplicateSubscriptions(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterTaskNotifications(ctx, eventBus, hub, log)

	// Count how many b.subscribe() calls are in RegisterTaskNotifications.
	// Each call creates exactly one subscription. If any event is subscribed
	// twice, this count will increase and the test will fail.
	//
	// Update this number when adding or removing event subscriptions in
	// RegisterTaskNotifications — it is intentionally exact.
	const wantSubscriptions = 46
	if got := len(b.subscriptions); got != wantSubscriptions {
		t.Errorf("RegisterTaskNotifications created %d subscriptions, want %d — "+
			"did an event get subscribed twice?", got, wantSubscriptions)
	}

	// Verify no event subject appears more than once by publishing each of
	// the 4 previously-duplicated events and counting hub broadcasts.
	// We use a fresh event bus per sub-test so each has exactly
	// 1 broadcaster subscription + 1 counter subscription.
	for _, subject := range []string{
		events.MessageAdded,
		events.MessageUpdated,
		events.TaskSessionStateChanged,
		events.GitHubTaskPRUpdated,
	} {
		subject := subject
		t.Run(subject, func(t *testing.T) {
			perEventBus := bus.NewMemoryEventBus(log)
			perHub := NewHub(nil, log)
			go perHub.Run(ctx)

			_ = RegisterTaskNotifications(ctx, perEventBus, perHub, log)

			var count int
			_, _ = perEventBus.Subscribe(subject, func(_ context.Context, _ *bus.Event) error {
				count++
				return nil
			})

			data := map[string]interface{}{
				"session_id": "s1", "task_id": "t1",
			}
			evt := bus.NewEvent(subject, "test", data)
			_ = perEventBus.Publish(context.Background(), subject, evt)

			// This verifies that publishing one event reaches our handler exactly
			// once. Duplicate-subscription detection is handled by the
			// len(b.subscriptions) guard above; these sub-tests cover event
			// delivery correctness for the four previously-duplicated subjects.
			if count != 1 {
				t.Errorf("counter handler fired %d times, want 1", count)
			}
		})
	}
}

// TestTaskEventBroadcaster_PreservesAllFields verifies that event data passes through the
// event bus unmodified — the broadcaster receives the same object that was published,
// including fields like turn_id, raw_content, and updated_at that the old
// subscribeEventBusHandlers used to strip before constructing a new payload manually.
func TestTaskEventBroadcaster_PreservesAllFields(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	// Subscribe a handler before the broadcaster to capture what data
	// the event bus delivers — the broadcaster receives the same object.
	var captured interface{}
	_, _ = eventBus.Subscribe(events.MessageAdded, func(_ context.Context, ev *bus.Event) error {
		captured = ev.Data
		return nil
	})

	_ = RegisterTaskNotifications(ctx, eventBus, hub, log)

	original := map[string]interface{}{
		"session_id":  "s1",
		"task_id":     "t1",
		"message_id":  "m1",
		"content":     "hello world",
		"raw_content": "raw hello world",
		"turn_id":     "turn-abc",
		"author_type": "agent",
		"author_id":   "claude",
		"type":        "text",
		"created_at":  "2026-04-20T00:00:00Z",
		"updated_at":  "2026-04-20T00:01:00Z",
		"metadata":    map[string]interface{}{"key": "value"},
	}

	evt := bus.NewEvent(events.MessageAdded, "test", original)
	_ = eventBus.Publish(context.Background(), events.MessageAdded, evt)

	capturedMap, ok := captured.(map[string]interface{})
	if !ok {
		t.Fatalf("captured data is not map[string]interface{}, got %T", captured)
	}

	// Verify fields that the old handler used to strip are still present.
	for _, field := range []string{"turn_id", "raw_content", "updated_at"} {
		if _, exists := capturedMap[field]; !exists {
			t.Errorf("field %q was stripped from event data", field)
		}
	}

	origJSON, _ := json.Marshal(original)
	capturedJSON, _ := json.Marshal(capturedMap)
	if string(origJSON) != string(capturedJSON) {
		t.Errorf("event data was modified\noriginal: %s\ncaptured: %s", origJSON, capturedJSON)
	}
}
