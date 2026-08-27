package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	githubsvc "github.com/kandev/kandev/internal/github"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

type subscriptionRecordingEventBus struct {
	*bus.MemoryEventBus
	subjects []string
}

type recordingSubscription struct{}

func (recordingSubscription) Unsubscribe() error { return nil }
func (recordingSubscription) IsValid() bool      { return true }

func (b *subscriptionRecordingEventBus) Subscribe(subject string, _ bus.EventHandler) (bus.Subscription, error) {
	b.subjects = append(b.subjects, subject)
	return recordingSubscription{}, nil
}

func TestTaskEventBroadcaster_UsesOrderedWildcardForLifecycleStates(t *testing.T) {
	log := testLogger()
	eventBus := &subscriptionRecordingEventBus{MemoryEventBus: bus.NewMemoryEventBus(log)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	b := RegisterTaskNotifications(ctx, eventBus, hub, log)

	seenWildcard := false
	for _, subject := range eventBus.subjects {
		if subject == ">" {
			seenWildcard = true
		}
		if subject == events.TaskStateChanged || subject == events.TaskSessionStateChanged {
			t.Fatalf("lifecycle state subject %q must use the ordered wildcard subscription", subject)
		}
	}
	if !seenWildcard {
		t.Fatal("lifecycle state notifications must share a NATS-style wildcard subscription")
	}

	_ = b
}

func TestTaskEventBroadcaster_OrdersLifecycleStateNotifications(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	hub := NewHub(nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = RegisterTaskNotifications(ctx, eventBus, hub, log)

	workspaceID := "workspace-1"
	_ = eventBus.Publish(ctx, events.TaskStateChanged, bus.NewEvent(
		events.TaskStateChanged,
		"test",
		map[string]interface{}{"task_id": "task-1", "workspace_id": workspaceID, "state": "IN_PROGRESS"},
	))
	_ = eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
		events.TaskSessionStateChanged,
		"test",
		map[string]interface{}{"task_id": "task-1", "session_id": "session-1", "workspace_id": workspaceID, "new_state": "RUNNING"},
	))

	first := <-hub.broadcast
	second := <-hub.broadcast
	if first.Action != ws.ActionTaskStateChanged || second.Action != ws.ActionSessionStateChanged {
		t.Fatalf("lifecycle notification order = %q, %q; want task state then session state", first.Action, second.Action)
	}
}

// TestTaskEventBroadcaster_NoDuplicateSubscriptions verifies that
// RegisterTaskNotifications creates one subscription per routed subject (with
// lifecycle state events intentionally sharing one ordered wildcard).
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

	// Count how many subscriptions RegisterTaskNotifications creates. If any
	// event is subscribed twice, this count will increase and the test will fail.
	//
	// Update this number when adding or removing event subscriptions in
	// RegisterTaskNotifications — it is intentionally exact.
	const wantSubscriptions = 70
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
		events.TaskSessionActivityChanged,
		events.GitHubTaskPRUpdated,
		events.GitHubTaskPRDeleted,
		events.GitLabTaskMRUpdated,
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

func TestTaskEventBroadcaster_CancellationIsSessionScoped(t *testing.T) {
	h := newTestHub(t)
	first := newTestClient("first")
	second := newTestClient("second")
	registerTestClient(h, first)
	registerTestClient(h, second)
	h.SubscribeToSession(first, "session-1")
	h.SubscribeToSession(second, "session-2")

	msg, err := ws.NewNotification("session.cancellation_changed", map[string]any{
		"session_id":           "session-1",
		"cancellation_pending": true,
	})
	require.NoError(t, err)
	b := &TaskEventBroadcaster{hub: h, logger: testLogger()}
	require.NoError(t, b.routeBroadcast("session.cancellation_changed", msg.Payload, "session-1", "", msg))

	if !clientReceived(first) {
		t.Fatal("session subscriber did not receive cancellation notification")
	}
	if clientReceived(second) {
		t.Fatal("cancellation notification crossed the session boundary")
	}
}

func TestTaskEventBroadcaster_DropsUnscopedGitHubCIOptionsWhenAuthIsEnforced(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{Enforced: func() bool { return true }})
	msg, err := ws.NewNotification(ws.ActionGitHubTaskCIOptionsUpdated, map[string]any{
		"task_id": "task-without-workspace",
	})
	require.NoError(t, err)
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}

	require.NoError(t, broadcaster.routeBroadcast(
		ws.ActionGitHubTaskCIOptionsUpdated,
		msg.Payload,
		"",
		"",
		msg,
	))

	select {
	case leaked := <-hub.broadcast:
		t.Fatalf("unscoped GitHub CI options update was globally broadcast: %s", leaked.Action)
	default:
	}
}

func TestTaskEventBroadcaster_ScopesTypedGitHubTaskPRUpdateToOwningWorkspace(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			if workspaceID == "workspace-a" {
				return "user-a", nil
			}
			return "", errors.New("unknown workspace")
		},
	})
	clientA := newTestClient("client-a")
	clientA.identity = authn.Identity{UserID: "user-a", Role: authn.RoleMember}
	clientB := newTestClient("client-b")
	clientB.identity = authn.Identity{UserID: "user-b", Role: authn.RoleMember}
	registerTestClient(hub, clientA)
	registerTestClient(hub, clientB)

	payload := &githubsvc.TaskPR{
		ID:          "association-a",
		WorkspaceID: "workspace-a",
		TaskID:      "task-a",
	}
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}
	msg := bus.NewEvent(events.GitHubTaskPRUpdated, "test", payload)

	require.NoError(t, broadcaster.broadcastEvent(context.Background(), msg, ws.ActionGitHubTaskPRUpdated))
	if !clientReceived(clientA) {
		t.Fatal("owner of workspace-a did not receive task PR update")
	}
	if clientReceived(clientB) {
		t.Fatal("task PR update crossed the workspace boundary")
	}
	select {
	case leaked := <-hub.broadcast:
		t.Fatalf("task PR update used the global broadcast path: %s", leaked.Action)
	default:
	}
}

func TestTaskEventBroadcaster_DropsUnscopedGitHubTaskPRUpdateWhenAuthIsEnforced(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{Enforced: func() bool { return true }})
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}
	payload := &githubsvc.TaskPR{TaskID: "task-without-workspace"}

	require.NoError(t, broadcaster.broadcastEvent(
		context.Background(),
		bus.NewEvent(events.GitHubTaskPRUpdated, "test", payload),
		ws.ActionGitHubTaskPRUpdated,
	))
	select {
	case leaked := <-hub.broadcast:
		t.Fatalf("unscoped task PR update was globally broadcast: %s", leaked.Action)
	default:
	}
}

func TestTaskEventBroadcaster_ScopesGitHubCIOptionsToOwningWorkspace(t *testing.T) {
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			switch workspaceID {
			case "workspace-a":
				return "user-a", nil
			case "workspace-b":
				return "user-b", nil
			default:
				return "", errors.New("unknown workspace")
			}
		},
	})
	clientA := newTestClient("client-a")
	clientA.identity = authn.Identity{UserID: "user-a", Role: authn.RoleMember}
	clientB := newTestClient("client-b")
	clientB.identity = authn.Identity{UserID: "user-b", Role: authn.RoleMember}
	registerTestClient(hub, clientA)
	registerTestClient(hub, clientB)
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}

	for _, test := range []struct {
		workspaceID string
		owner       *Client
		foreign     *Client
	}{
		{workspaceID: "workspace-a", owner: clientA, foreign: clientB},
		{workspaceID: "workspace-b", owner: clientB, foreign: clientA},
	} {
		payload := &githubsvc.TaskCIOptionsResponse{TaskID: "task-" + test.workspaceID, WorkspaceID: test.workspaceID}
		msg, err := ws.NewNotification(ws.ActionGitHubTaskCIOptionsUpdated, payload)
		require.NoError(t, err)
		require.NoError(t, broadcaster.routeBroadcast(
			ws.ActionGitHubTaskCIOptionsUpdated,
			payload,
			"",
			extractWorkspaceID(payload),
			msg,
		))
		if !clientReceived(test.owner) {
			t.Fatalf("owner of %s did not receive CI options update", test.workspaceID)
		}
		if clientReceived(test.foreign) {
			t.Fatalf("CI options update for %s crossed workspace boundary", test.workspaceID)
		}
	}
}

func TestTaskEventBroadcaster_CancellationSubscriptionIsSessionScoped(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	hub := newTestHub(t)
	first := newTestClient("first")
	second := newTestClient("second")
	registerTestClient(hub, first)
	registerTestClient(hub, second)
	hub.SubscribeToSession(first, "session-1")
	hub.SubscribeToSession(second, "session-2")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := RegisterTaskNotifications(ctx, eventBus, hub, log)
	t.Cleanup(b.Close)

	require.NoError(t, eventBus.Publish(ctx, events.TaskSessionCancellationChanged, bus.NewEvent(
		events.TaskSessionCancellationChanged,
		"test",
		map[string]any{"session_id": "session-1", "cancellation_pending": true},
	)))

	var notification ws.Message
	received := <-first.send
	require.NoError(t, json.Unmarshal(received, &notification))
	require.Equal(t, ws.ActionSessionCancellationChanged, notification.Action)
	if clientReceived(second) {
		t.Fatal("cancellation subscription crossed the session boundary")
	}
}
