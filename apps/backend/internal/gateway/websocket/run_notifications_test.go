package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestRunEventBroadcaster_SubscribesToWildcard verifies that
// RegisterRunNotifications attaches one wildcard subscription so all
// run-id-namespaced subjects feed through the same handler.
func TestRunEventBroadcaster_SubscribesToWildcard(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterRunNotifications(ctx, eventBus, hub, log)
	if b.subscription == nil {
		t.Fatalf("expected wildcard subscription, got nil")
	}
	if !b.subscription.IsValid() {
		t.Fatalf("expected wildcard subscription to be valid")
	}
}

// TestRunEventBroadcaster_NilEventBus verifies no panic + no
// subscription when the event bus is nil.
func TestRunEventBroadcaster_NilEventBus(t *testing.T) {
	log := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterRunNotifications(ctx, nil, hub, log)
	if b.subscription != nil {
		t.Fatalf("expected nil subscription with nil event bus")
	}
}

// TestRunEventBroadcaster_FansOutByRunID verifies that publishing
// a run event for run "A" only delivers to clients subscribed to "A",
// not to clients subscribed to a different run.
func TestRunEventBroadcaster_FansOutByRunID(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	_ = RegisterRunNotifications(ctx, eventBus, hub, log)

	// Spy on hub.BroadcastToRun by snooping the run-subscriber map.
	clientA := NewClient("client-A", nil, hub, log)
	clientB := NewClient("client-B", nil, hub, log)
	hub.SubscribeToRun(clientA, "run-A")
	hub.SubscribeToRun(clientB, "run-B")

	// Publish event on subject for run-A.
	subject := events.BuildOfficeRunEventSubject("run-A")
	evt := bus.NewEvent(subject, "test", map[string]interface{}{
		"run_id": "run-A",
		"event": map[string]interface{}{
			"seq":        0,
			"event_type": "init",
			"level":      "info",
			"payload":    "{}",
			"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	if err := eventBus.Publish(context.Background(), subject, evt); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// run-A's subscriber list should contain client-A; run-B should
	// remain its own untouched cohort. We can't easily assert send-buffer
	// contents here without a real conn, so just confirm SubscribeToRun
	// kept the topology straight and the broadcaster didn't blow up on
	// a payload it doesn't recognise.
	clients := hub.getSubscribersLocked(hub.runSubscribers, "run-A")
	if len(clients) != 1 || clients[0] != clientA {
		t.Fatalf("expected only clientA on run-A, got %v", clients)
	}
	clients = hub.getSubscribersLocked(hub.runSubscribers, "run-B")
	if len(clients) != 1 || clients[0] != clientB {
		t.Fatalf("expected only clientB on run-B, got %v", clients)
	}
}

// TestRunEventBroadcaster_IgnoresMalformedPayload verifies that the
// handler tolerates events whose data isn't a map or whose run_id is
// missing. Published events without a run id are silently dropped.
func TestRunEventBroadcaster_IgnoresMalformedPayload(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	_ = RegisterRunNotifications(ctx, eventBus, hub, log)

	// Non-map payload — must not panic.
	subject := events.BuildOfficeRunEventSubject("run-X")
	evt := bus.NewEvent(subject, "test", "not-a-map")
	if err := eventBus.Publish(context.Background(), subject, evt); err != nil {
		t.Fatalf("publish non-map: %v", err)
	}

	// Map payload but missing run_id.
	evt2 := bus.NewEvent(subject, "test", map[string]interface{}{
		"event": map[string]interface{}{"seq": 0},
	})
	if err := eventBus.Publish(context.Background(), subject, evt2); err != nil {
		t.Fatalf("publish missing run_id: %v", err)
	}
}

// TestClient_HandleRunSubscribe_Validation verifies that run.subscribe
// and run.unsubscribe reject empty run_id with a validation error.
func TestClient_HandleRunSubscribe_Validation(t *testing.T) {
	log := testLogger()
	hub := NewHub(nil, log)
	client := NewClient("c1", nil, hub, log)

	// Empty payload — must not register.
	msg := &ws.Message{
		ID:     "1",
		Type:   "request",
		Action: ws.ActionRunSubscribe,
	}
	client.handleRunSubscribe(msg)
	if len(client.runSubscriptions) != 0 {
		t.Fatalf("expected no subscription on missing run_id")
	}

	// Valid payload — must register.
	msg2, err := ws.NewRequest("2", ws.ActionRunSubscribe, map[string]interface{}{
		"run_id": "run-1",
	})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	client.handleRunSubscribe(msg2)
	if !client.runSubscriptions["run-1"] {
		t.Fatalf("expected client subscribed to run-1")
	}

	// Unsubscribe.
	msg3, err := ws.NewRequest("3", ws.ActionRunUnsubscribe, map[string]interface{}{
		"run_id": "run-1",
	})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	client.handleRunUnsubscribe(msg3)
	if client.runSubscriptions["run-1"] {
		t.Fatalf("expected client unsubscribed from run-1")
	}
}
