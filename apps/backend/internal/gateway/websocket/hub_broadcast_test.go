package websocket

import (
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// registerTestClient adds a client to the hub's connected set so broadcasts can
// reach it (BroadcastToSession only iterates subscriber/focus maps, but the
// client must be a live connection in real usage).
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

// TestBroadcastToSession_ReachesFocusedClient guards the resume regression:
// a client that is focused on a session but whose ref-counted session.subscribe
// was dropped (subscriber_count 0) must still receive session-scoped broadcasts
// such as session.message.updated. Focus is the stable "actively viewing" signal;
// the subscribe ref-count churns to 0 during task-switch/resume.
func TestBroadcastToSession_ReachesFocusedClient(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c1")
	registerTestClient(h, c)

	// Focused but NOT subscribed — mirrors the dropped-subscription resume race.
	h.FocusSession(c, "sess-1")

	msg, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{"session_id": "sess-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("sess-1", msg)

	if !clientReceived(c) {
		t.Fatal("focused-but-unsubscribed client did not receive session broadcast")
	}
}

// TestBroadcastToSession_ReachesSubscribedClient is the baseline: a subscribed
// (sidebar/background) client still receives broadcasts after the union change.
func TestBroadcastToSession_ReachesSubscribedClient(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c1")
	registerTestClient(h, c)

	h.SubscribeToSession(c, "sess-1")

	msg, err := ws.NewNotification(ws.ActionSessionMessageAdded, map[string]any{"session_id": "sess-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("sess-1", msg)

	if !clientReceived(c) {
		t.Fatal("subscribed client did not receive session broadcast")
	}
}

// TestBroadcastToSession_NotDeliveredToUninterestedClient ensures the union does
// not over-deliver: a client neither subscribed nor focused gets nothing.
func TestBroadcastToSession_NotDeliveredToUninterestedClient(t *testing.T) {
	h := newTestHub(t)
	focused := newTestClient("focused")
	other := newTestClient("other")
	registerTestClient(h, focused)
	registerTestClient(h, other)

	h.FocusSession(focused, "sess-1")

	msg, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{"session_id": "sess-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("sess-1", msg)

	if !clientReceived(focused) {
		t.Fatal("focused client did not receive broadcast")
	}
	if clientReceived(other) {
		t.Fatal("uninterested client should not receive broadcast")
	}
}

// TestBroadcastToSession_DeliversOnceToSubscribedAndFocusedClient ensures a
// client that is BOTH subscribed and focused receives exactly one copy (the
// union must dedupe).
func TestBroadcastToSession_DeliversOnceToSubscribedAndFocusedClient(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c1")
	registerTestClient(h, c)

	h.SubscribeToSession(c, "sess-1")
	h.FocusSession(c, "sess-1")

	msg, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{"session_id": "sess-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("sess-1", msg)

	if !clientReceived(c) {
		t.Fatal("subscribed+focused client did not receive broadcast")
	}
	if clientReceived(c) {
		t.Fatal("subscribed+focused client received a duplicate broadcast")
	}
}

func TestBroadcastToUserReportsWhetherAFrameWasQueued(t *testing.T) {
	h := newTestHub(t)
	msg, err := ws.NewNotification("system.update_available", map[string]any{"occurrence_id": "v1.2.3"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if h.BroadcastToUser("user-1", msg) {
		t.Fatal("broadcast without a user subscriber must report no queued recipient")
	}

	c := newTestClient("c1")
	registerTestClient(h, c)
	h.SubscribeToUser(c, "user-1")
	if !h.BroadcastToUser("user-1", msg) {
		t.Fatal("broadcast with a user subscriber must report a queued recipient")
	}
}

func TestBroadcastToUserReportsFalseWhenSubscriberBufferIsFull(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c1")
	registerTestClient(h, c)
	h.SubscribeToUser(c, "user-1")
	for index := 0; index < cap(c.send); index++ {
		c.send <- []byte("queued")
	}

	msg, err := ws.NewNotification("system.update_available", map[string]any{"occurrence_id": "v1.2.3"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if h.BroadcastToUser("user-1", msg) {
		t.Fatal("broadcast with a full subscriber buffer must report no queued recipient")
	}
}

func TestBroadcastToUserReportsFalseWhenSubscriberIsClosed(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("c1")
	registerTestClient(h, c)
	h.SubscribeToUser(c, "user-1")
	c.closeSend()

	msg, err := ws.NewNotification("system.update_available", map[string]any{"occurrence_id": "v1.2.3"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if h.BroadcastToUser("user-1", msg) {
		t.Fatal("broadcast to a closed subscriber must report no queued recipient")
	}
}

func TestSendToIdentityTargetsConnectedClientsWithoutSubscription(t *testing.T) {
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
	if !clientReceived(first) || !clientReceived(second) {
		t.Fatal("both matching identity clients must receive the notification")
	}
	if clientReceived(other) {
		t.Fatal("a different identity must not receive the notification")
	}
}
