package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func newAccessTestHub(t *testing.T) *Hub {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	hub := NewHub(ws.NewDispatcher(), log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	return hub
}

func registerAccessClient(t *testing.T, hub *Hub, id string, identity authn.Identity) *Client {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	client := NewClient(id, identity, nil, hub, log)
	hub.Register(client)
	waitForClientCount(t, hub, id)
	return client
}

// waitForClientCount waits until the hub has processed the register message
// for the given client (Register goes through the Run loop channel).
func waitForClientCount(t *testing.T, hub *Hub, clientID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		found := false
		for client := range hub.clients {
			if client.ID == clientID {
				found = true
				break
			}
		}
		hub.mu.RUnlock()
		if found {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("client %s never registered", clientID)
}

func receivedActions(client *Client) []string {
	actions := []string{}
	for {
		select {
		case raw := <-client.controlSend:
			var msg ws.Message
			if err := json.Unmarshal(raw, &msg); err == nil {
				actions = append(actions, msg.Action)
			}
		case raw := <-client.send:
			var msg ws.Message
			if err := json.Unmarshal(raw, &msg); err == nil {
				actions = append(actions, msg.Action)
			}
		default:
			return actions
		}
	}
}

func TestBroadcastToWorkspaceRoutesByOwner(t *testing.T) {
	hub := newAccessTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			switch workspaceID {
			case "ws-a":
				return "user-a", nil
			case "ws-unowned":
				return "", nil
			}
			return "", errors.New("unknown workspace")
		},
	})

	clientA := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	clientB := registerAccessClient(t, hub, "b", authn.Identity{UserID: "user-b", Role: authn.RoleMember})
	clientSynthetic := registerAccessClient(t, hub, "s", authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true})

	notify := func(action string) *ws.Message {
		msg, err := ws.NewNotification(action, map[string]interface{}{"workspace_id": "ws-a"})
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}

	// Owned workspace: owner + synthetic receive, the other user does not.
	hub.BroadcastToWorkspace("ws-a", notify("task.created"))
	if got := receivedActions(clientA); len(got) != 1 {
		t.Fatalf("owner received %v, want 1 message", got)
	}
	if got := receivedActions(clientB); len(got) != 0 {
		t.Fatalf("foreign user received %v, want none — WS LEAK", got)
	}
	if got := receivedActions(clientSynthetic); len(got) != 1 {
		t.Fatalf("synthetic client received %v, want 1 message", got)
	}

	// Unowned workspace falls back to broadcast-to-all (async via Run loop).
	hub.BroadcastToWorkspace("ws-unowned", notify("task.updated"))
	waitForMessage(t, clientB)

	// Resolver errors also fall back to broadcast (never drop events).
	hub.BroadcastToWorkspace("ws-mystery", notify("task.updated"))
	waitForMessage(t, clientB)

	// Empty workspace ID = instance-wide event = everyone.
	hub.BroadcastToWorkspace("", notify("executor.created"))
	waitForMessage(t, clientA)
}

func waitForMessage(t *testing.T, client *Client) {
	t.Helper()
	select {
	case <-client.send:
	case <-time.After(2 * time.Second):
		t.Fatalf("client %s did not receive expected broadcast", client.ID)
	}
}

func TestSubscriptionChecksDenyForeignTopics(t *testing.T) {
	hub := newAccessTestHub(t)
	denied := errors.New("denied")
	hub.setAuthPolicy(AuthPolicy{
		Subscriptions: SubscriptionAccessPolicy{
			Task: func(ctx context.Context, taskID string) error {
				identity, _ := authn.IdentityFromContext(ctx)
				if taskID == "task-b" && identity.UserID != "user-b" {
					return denied
				}
				return nil
			},
			Session: func(ctx context.Context, sessionID string) error {
				identity, _ := authn.IdentityFromContext(ctx)
				if sessionID == "sess-b" && identity.UserID != "user-b" {
					return denied
				}
				return nil
			},
		},
	})
	clientA := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})

	subscribe := func(action string, payload map[string]interface{}) *ws.Message {
		raw, _ := json.Marshal(payload)
		return &ws.Message{ID: "1", Type: ws.MessageTypeRequest, Action: action, Payload: raw}
	}

	clientA.handleSubscribe(subscribe(ws.ActionTaskSubscribe, map[string]interface{}{"task_id": "task-b"}))
	assertErrorResponse(t, clientA, "task subscribe")
	if len(hub.getSubscribersLocked(hub.taskSubscribers, "task-b")) != 0 {
		t.Fatal("denied task subscription must not register")
	}

	clientA.handleSessionSubscribe(subscribe(ws.ActionSessionSubscribe, map[string]interface{}{"session_id": "sess-b"}))
	assertErrorResponse(t, clientA, "session subscribe")
	if len(hub.getSubscribersLocked(hub.sessionSubscribers, "sess-b")) != 0 {
		t.Fatal("denied session subscription must not register")
	}

	// Own topics still work.
	clientA.handleSubscribe(subscribe(ws.ActionTaskSubscribe, map[string]interface{}{"task_id": "task-a"}))
	assertSuccessResponse(t, clientA, "own task subscribe")
	if len(hub.getSubscribersLocked(hub.taskSubscribers, "task-a")) != 1 {
		t.Fatal("allowed task subscription must register")
	}
}

func TestUserSubscribeScopedToOwnUser(t *testing.T) {
	hub := newAccessTestHub(t)
	client := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})

	raw, _ := json.Marshal(map[string]interface{}{"user_id": "user-b"})
	client.handleUserSubscribe(&ws.Message{ID: "1", Type: ws.MessageTypeRequest, Action: ws.ActionUserSubscribe, Payload: raw})
	assertErrorResponse(t, client, "foreign user subscribe")

	// Empty user_id defaults to the client's own user.
	raw, _ = json.Marshal(map[string]interface{}{})
	client.handleUserSubscribe(&ws.Message{ID: "2", Type: ws.MessageTypeRequest, Action: ws.ActionUserSubscribe, Payload: raw})
	assertSuccessResponse(t, client, "own user subscribe")
	if len(hub.getSubscribersLocked(hub.userSubscribers, "user-a")) != 1 {
		t.Fatal("own-user subscription must land on the client's user topic")
	}
}

func assertErrorResponse(t *testing.T, client *Client, label string) {
	t.Helper()
	select {
	case raw := <-client.controlSend:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type != ws.MessageTypeError {
			t.Fatalf("%s: got %s frame, want error", label, msg.Type)
		}
	case raw := <-client.send:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type != ws.MessageTypeError {
			t.Fatalf("%s: got %s frame, want error", label, msg.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: no response frame", label)
	}
}

func assertSuccessResponse(t *testing.T, client *Client, label string) {
	t.Helper()
	select {
	case raw := <-client.controlSend:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type == ws.MessageTypeError {
			t.Fatalf("%s: got error frame: %s", label, string(raw))
		}
	case raw := <-client.send:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("%s: bad frame: %v", label, err)
		}
		if msg.Type == ws.MessageTypeError {
			t.Fatalf("%s: got error frame: %s", label, string(raw))
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: no response frame", label)
	}
}

// Regression: workspace.* event payloads carry the workspace ID under "id"
// (the payload IS the workspace DTO), not "workspace_id". The first
// implementation missed this and broadcast admin workspace updates to every
// user — caught by the auth E2E segregation spec.
func TestExtractWorkspaceIDFieldShapes(t *testing.T) {
	if got := extractWorkspaceID(map[string]interface{}{"workspace_id": "ws-1"}); got != "ws-1" {
		t.Fatalf("workspace_id key: %q", got)
	}
	if got := extractWorkspaceID(map[string]interface{}{"id": "ws-1"}); got != "" {
		t.Fatalf("bare id must NOT be treated as workspace context for generic events: %q", got)
	}
	if got := extractStringField(map[string]interface{}{"id": "ws-1"}, "id"); got != "ws-1" {
		t.Fatalf("extractStringField: %q", got)
	}
	if got := extractWorkspaceID("not-a-map"); got != "" {
		t.Fatalf("non-map payload: %q", got)
	}
}
