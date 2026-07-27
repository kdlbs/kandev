package websocket

import (
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestHub_UserSubscriptionListenerRunsAfterRegistrationOutsideHubLock(t *testing.T) {
	h := newTestHub(t)
	client := newTestClient("c1")

	called := false
	h.AddUserSubscriptionListener(func(userID string) {
		called = true
		msg, err := ws.NewNotification("system.update_available", map[string]any{"occurrence_id": "v1.0.1"})
		if err != nil {
			t.Fatalf("new notification: %v", err)
		}
		if !h.BroadcastToUser(userID, msg) {
			t.Fatal("subscription listener ran before user registration")
		}
	})

	h.SubscribeToUser(client, "default")

	if !called {
		t.Fatal("user subscription listener was not called")
	}
	if !clientReceived(client) {
		t.Fatal("listener broadcast did not reach newly subscribed client")
	}
}
