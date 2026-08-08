package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestAgentRuntimeNotificationsBroadcastAndReplaySanitizedState(t *testing.T) {
	hub := newTestHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)

	eventBus := bus.NewMemoryEventBus(testLoggerForUserNotifications(t))
	t.Cleanup(eventBus.Close)
	payload := map[string]any{"status": "unavailable", "reason": "agentctl_exited"}
	broadcaster := RegisterAgentRuntimeNotifications(ctx, eventBus, hub, func() (any, bool) {
		return payload, true
	}, testLoggerForUserNotifications(t))
	t.Cleanup(broadcaster.Close)

	liveClient := newTestClient("live")
	registerTestClient(hub, liveClient)
	if err := eventBus.Publish(ctx, events.AgentRuntimeAvailabilityChanged,
		bus.NewEvent(events.AgentRuntimeAvailabilityChanged, "test", payload)); err != nil {
		t.Fatalf("publish availability event: %v", err)
	}
	liveMessage := readNotification(t, liveClient)
	if liveMessage.Action != ws.ActionSystemAgentRuntimeStatusChanged {
		t.Fatalf("live action = %q, want %q", liveMessage.Action, ws.ActionSystemAgentRuntimeStatusChanged)
	}

	replayClient := newTestClient("replay")
	registerTestClient(hub, replayClient)
	hub.SubscribeToUser(replayClient, "user-1")
	replayMessage := readNotification(t, replayClient)
	if replayMessage.Action != ws.ActionSystemAgentRuntimeStatusChanged {
		t.Fatalf("replay action = %q, want %q", replayMessage.Action, ws.ActionSystemAgentRuntimeStatusChanged)
	}
	var replayPayload map[string]any
	if err := json.Unmarshal(replayMessage.Payload, &replayPayload); err != nil {
		t.Fatalf("decode replay payload: %v", err)
	}
	if replayPayload["status"] != "unavailable" || replayPayload["reason"] != "agentctl_exited" {
		t.Fatalf("replay payload = %#v", replayPayload)
	}
}

func TestSystemEventBroadcasterCloseIsIdempotent(t *testing.T) {
	log := testLoggerForUserNotifications(t)
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hub := newTestHub(t)
	broadcaster := RegisterSystemNotifications(ctx, eventBus, hub, log)

	var wg sync.WaitGroup
	for index := 0; index < 16; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			broadcaster.Close()
		}()
	}
	wg.Wait()

	if len(broadcaster.subscriptions) != 0 {
		t.Fatalf("subscriptions after concurrent Close = %d, want 0", len(broadcaster.subscriptions))
	}
	eventBus.Close()
}

func readNotification(t *testing.T, client *Client) ws.Message {
	t.Helper()
	select {
	case frame := <-client.send:
		var message ws.Message
		if err := json.Unmarshal(frame, &message); err != nil {
			t.Fatalf("decode websocket frame: %v", err)
		}
		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket notification")
		return ws.Message{}
	}
}
