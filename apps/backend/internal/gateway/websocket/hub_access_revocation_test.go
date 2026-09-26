package websocket

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestTaskBroadcastEvictsSubscriberAfterAccessRevocation(t *testing.T) {
	hub := newTestHub(t)
	allowed := true
	hub.setAuthPolicy(AuthPolicy{Subscriptions: SubscriptionAccessPolicy{
		Task: func(ctx context.Context, _ string) error {
			identity, _ := authn.IdentityFromContext(ctx)
			if identity.UserID == "revoked-user" && !allowed {
				return errors.New("access revoked")
			}
			return nil
		},
	}})
	client := newTestClient("revoked-task-client")
	client.hub = hub
	client.identity = authn.Identity{UserID: "revoked-user", Role: authn.RoleMember}
	hub.SubscribeToTask(client, "task-1")
	allowed = false

	message, err := ws.NewNotification(ws.ActionTaskUpdated, map[string]string{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}
	hub.BroadcastToTask("task-1", message)

	assertNoMoreFrames(t, client)
	if client.subscriptions["task-1"] {
		t.Fatal("revoked client retained its task subscription")
	}
	if clients := hub.getSubscribersLocked(hub.taskSubscribers, "task-1"); len(clients) != 0 {
		t.Fatalf("revoked task subscribers = %d, want 0", len(clients))
	}
}

func TestSessionBroadcastEvictsSubscriberAndFocusAfterAccessRevocation(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{Subscriptions: SubscriptionAccessPolicy{
		Session: func(context.Context, string) error { return errors.New("access revoked") },
	}})
	client := newTestClient("revoked-session-client")
	client.hub = hub
	client.identity = authn.Identity{UserID: "revoked-user", Role: authn.RoleMember}
	hub.SubscribeToSession(client, "session-1")
	hub.FocusSession(client, "session-1")

	message, err := ws.NewNotification(ws.ActionSessionMessageAdded, map[string]string{"session_id": "session-1"})
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}
	hub.BroadcastToSession("session-1", message)

	assertNoMoreFrames(t, client)
	if client.sessionSubscriptions["session-1"] || client.sessionFocus["session-1"] {
		t.Fatal("revoked client retained its session subscription or focus")
	}
	if clients := hub.getSessionRecipientsLocked("session-1"); len(clients) != 0 {
		t.Fatalf("revoked session recipients = %d, want 0", len(clients))
	}
}
