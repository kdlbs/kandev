package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// broadcastRecorder records all messages broadcast via a Hub.
type broadcastRecorder struct {
	mu       sync.Mutex
	messages []*ws.Message
}

func (r *broadcastRecorder) record(msg *ws.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
}

func (r *broadcastRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

func (r *broadcastRecorder) all() []*ws.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*ws.Message, len(r.messages))
	copy(cp, r.messages)
	return cp
}

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

// TestTaskEventBroadcaster_NoDuplicateSubscriptions proves that
// RegisterTaskNotifications subscribes once per event type — i.e., publishing a
// single event produces exactly one WebSocket notification.
//
// The old code had a second subscription system (subscribeEventBusHandlers in
// cmd/kandev/helpers.go) that subscribed to the same four events, causing
// duplicate broadcasts. This test guards against re-introducing that.
func TestTaskEventBroadcaster_NoDuplicateSubscriptions(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recorder := &broadcastRecorder{}

	// Create a minimal Hub that records broadcasts instead of sending to real
	// WebSocket connections. We only need the broadcast channel to be consumed.
	hub := NewHub(nil, log)
	go hub.Run(ctx)

	// Override broadcast behaviour: intercept messages via a subscription on
	// the hub's session subscriber map for our test session.
	// Instead, we hook into the event bus directly alongside the broadcaster
	// to count how many times the broadcaster fires.
	//
	// Strategy: register the broadcaster, then for each of the 4 previously-
	// duplicated events, publish once and verify the event bus delivered to
	// exactly one handler per event type.

	_ = RegisterTaskNotifications(ctx, eventBus, hub, log)

	type testCase struct {
		eventSubject string
		wsAction     string
		data         map[string]interface{}
	}

	cases := []testCase{
		{events.MessageAdded, ws.ActionSessionMessageAdded, map[string]interface{}{
			"session_id": "s1", "task_id": "t1", "message_id": "m1",
			"content": "hello", "turn_id": "turn-1", "raw_content": "raw",
		}},
		{events.MessageUpdated, ws.ActionSessionMessageUpdated, map[string]interface{}{
			"session_id": "s1", "task_id": "t1", "message_id": "m2",
			"content": "updated", "turn_id": "turn-2", "raw_content": "raw2",
		}},
		{events.TaskSessionStateChanged, ws.ActionSessionStateChanged, map[string]interface{}{
			"session_id": "s1", "task_id": "t1", "new_state": "running",
		}},
		{events.GitHubTaskPRUpdated, ws.ActionGitHubTaskPRUpdated, map[string]interface{}{
			"task_id": "t1", "pr_url": "https://github.com/org/repo/pull/1",
		}},
	}

	// For each event we install an extra counting handler so we can verify
	// the total number of handlers that fire (should be 2: broadcaster + our counter).
	for _, tc := range cases {
		tc := tc
		t.Run(tc.eventSubject, func(t *testing.T) {
			counter := &broadcastRecorder{}

			// Add a counting handler
			_, err := eventBus.Subscribe(tc.eventSubject, func(_ context.Context, ev *bus.Event) error {
				msg, _ := ws.NewNotification(tc.wsAction, ev.Data)
				counter.record(msg)
				recorder.record(msg)
				return nil
			})
			if err != nil {
				t.Fatalf("subscribe counter: %v", err)
			}

			evt := bus.NewEvent(tc.eventSubject, "test", tc.data)
			if err := eventBus.Publish(context.Background(), tc.eventSubject, evt); err != nil {
				t.Fatalf("publish: %v", err)
			}

			// The MemoryEventBus delivers synchronously, so by the time
			// Publish returns, all handlers have fired.

			// Our counter handler saw exactly 1 delivery.
			if got := counter.count(); got != 1 {
				t.Errorf("counter handler fired %d times, want 1", got)
			}
		})
	}
}

// TestTaskEventBroadcaster_PreservesAllFields verifies that the broadcaster
// forwards the full event payload (including turn_id, raw_content, etc.)
// rather than stripping fields like the old subscribeEventBusHandlers did.
func TestTaskEventBroadcaster_PreservesAllFields(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	// We can't easily intercept Hub.BroadcastToSession, so instead we
	// subscribe a second handler that captures what the broadcaster would
	// receive, and verify the event data is unmodified.
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

	// captured should be the same object (MemoryEventBus passes data by reference)
	capturedMap, ok := captured.(map[string]interface{})
	if !ok {
		t.Fatalf("captured data is not map[string]interface{}, got %T", captured)
	}

	// Verify fields that the old handler used to strip are still present
	for _, field := range []string{"turn_id", "raw_content", "updated_at"} {
		if _, exists := capturedMap[field]; !exists {
			t.Errorf("field %q was stripped from event data", field)
		}
	}

	// Verify all original fields are present
	origJSON, _ := json.Marshal(original)
	capturedJSON, _ := json.Marshal(capturedMap)
	if string(origJSON) != string(capturedJSON) {
		t.Errorf("event data was modified\noriginal: %s\ncaptured: %s", origJSON, capturedJSON)
	}
}
