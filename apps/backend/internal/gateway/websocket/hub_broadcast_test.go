package websocket

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func registerTestClient(h *Hub, c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

func clientReceived(c *Client) bool {
	select {
	case <-c.send:
		return true
	default:
		return false
	}
}

func TestBroadcastToSessionReachesFocusedAndSubscribedClients(t *testing.T) {
	h := newTestHub(t)
	focused := newTestClient("focused")
	subscribed := newTestClient("subscribed")
	other := newTestClient("other")
	registerTestClient(h, focused)
	registerTestClient(h, subscribed)
	registerTestClient(h, other)
	h.FocusSession(focused, "sess-1")
	h.SubscribeToSession(subscribed, "sess-1")

	msg, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{"session_id": "sess-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("sess-1", msg)

	if !clientReceived(focused) || !clientReceived(subscribed) {
		t.Fatal("focused and subscribed clients must receive the broadcast")
	}
	if clientReceived(other) {
		t.Fatal("uninterested client received the broadcast")
	}
}

func TestSessionLaunchWarningReplaysToLateSubscriber(t *testing.T) {
	h := newTestHub(t)
	warning, err := ws.NewNotification(ws.ActionSessionLaunchWarning, map[string]any{
		"session_id": "sess-1",
		"host":       "10.0.0.5",
	})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.rememberAndBroadcastSessionLaunchWarning("sess-1", warning)

	late := newTestClient("late")
	registerTestClient(h, late)
	if !h.SubscribeToSession(late, "sess-1") {
		t.Fatal("late subscriber should create a new membership")
	}

	select {
	case data := <-late.send:
		var got ws.Message
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode replay: %v", err)
		}
		if got.Action != ws.ActionSessionLaunchWarning {
			t.Fatalf("replayed action = %q, want %q", got.Action, ws.ActionSessionLaunchWarning)
		}
	default:
		t.Fatal("late subscriber did not receive the cached launch warning")
	}

	h.clearSessionLaunchWarning("sess-1")
	newClient := newTestClient("new")
	registerTestClient(h, newClient)
	h.SubscribeToSession(newClient, "sess-1")
	if clientReceived(newClient) {
		t.Fatal("cleared launch warning was replayed")
	}
}

func TestBroadcastToSessionDeduplicatesFocusedSubscription(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c1")
	registerTestClient(h, c)
	h.SubscribeToSession(c, "sess-1")
	h.FocusSession(c, "sess-1")

	msg, err := ws.NewNotification(ws.ActionSessionMessageAdded, map[string]any{"session_id": "sess-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("sess-1", msg)

	if !clientReceived(c) || clientReceived(c) {
		t.Fatal("client must receive exactly one broadcast")
	}
}

func TestBroadcastToUserReportsQueueResult(t *testing.T) {
	h := newTestHub(t)
	msg, err := ws.NewNotification("system.update_available", map[string]any{"occurrence_id": "v1.2.3"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if h.BroadcastToUser("user-1", msg) {
		t.Fatal("broadcast without subscribers must report false")
	}
	c := newTestClient("c1")
	registerTestClient(h, c)
	h.SubscribeToUser(c, "user-1")
	if !h.BroadcastToUser("user-1", msg) {
		t.Fatal("broadcast with a subscriber must report true")
	}
}

func TestSendToIdentityTargetsMatchingClients(t *testing.T) {
	h := newTestHub(t)
	first := newTestClient("first")
	first.identity = authn.Identity{UserID: "user-1"}
	second := newTestClient("second")
	second.identity = authn.Identity{UserID: "user-1"}
	other := newTestClient("other")
	other.identity = authn.Identity{UserID: "user-2"}
	registerTestClient(h, first)
	registerTestClient(h, second)
	registerTestClient(h, other)

	msg, err := ws.NewNotification("system.logs.capture_requested", map[string]any{"bundle_id": "bundle-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if got := h.SendToIdentity("user-1", msg); got != 2 {
		t.Fatalf("queued recipients = %d, want 2", got)
	}
	if !clientReceived(first) || !clientReceived(second) || clientReceived(other) {
		t.Fatal("identity broadcast reached the wrong clients")
	}
}
