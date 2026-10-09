package websocket

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.3
//
// events.ExecutorReachabilityChanged must reach WS clients as
// ws.ActionExecutorReachabilityChanged. Executor events carry no
// workspace_id, so — like ExecutorUpdated before it — this rides the
// broadcaster's trailing default case (global broadcast) rather than a
// dedicated routeBroadcast switch entry.
func TestTaskEventBroadcaster_BroadcastsExecutorReachabilityChanged(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	hub := NewHub(nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = RegisterTaskNotifications(ctx, eventBus, hub, log)

	payload := map[string]interface{}{"executor_id": "exec-1", "state": "unreachable", "reason": "network"}
	_ = eventBus.Publish(ctx, events.ExecutorReachabilityChanged, bus.NewEvent(
		events.ExecutorReachabilityChanged, "test", payload,
	))

	msg := <-hub.broadcast
	if msg.Action != ws.ActionExecutorReachabilityChanged {
		t.Fatalf("broadcast action = %q, want %q", msg.Action, ws.ActionExecutorReachabilityChanged)
	}
}
