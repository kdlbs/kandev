package websocket

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestOfficeEventBroadcaster_SubscriptionCount verifies that
// RegisterOfficeNotifications creates exactly one subscription per event type.
func TestOfficeEventBroadcaster_SubscriptionCount(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterOfficeNotifications(ctx, eventBus, hub, nil, log)

	// One subscription per event type subscribed in RegisterOfficeNotifications.
	// Update this count when adding/removing event subscriptions.
	const wantSubscriptions = 20
	if got := len(b.subscriptions); got != wantSubscriptions {
		t.Errorf("RegisterOfficeNotifications created %d subscriptions, want %d",
			got, wantSubscriptions)
	}
}

// TestOfficeEventBroadcaster_BroadcastsEvent verifies that publishing an
// office event on the bus results in a hub.Broadcast call.
func TestOfficeEventBroadcaster_BroadcastsEvent(t *testing.T) {
	log := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCases := []struct {
		subject string
	}{
		{events.OfficeCommentCreated},
		{events.OfficeApprovalCreated},
		{events.OfficeTaskStatusChanged},
		{events.AgentCompleted},
		{events.AgentFailed},
		{events.TaskMoved},
	}

	for _, tc := range testCases {
		t.Run(tc.subject, func(t *testing.T) {
			eb := bus.NewMemoryEventBus(log)
			hub := NewHub(nil, log)
			go hub.Run(ctx)

			_ = RegisterOfficeNotifications(ctx, eb, hub, nil, log)

			// Track whether the office broadcaster's handler ran by counting
			// all subscribers on this subject (broadcaster + our counter).
			var handlerCalled int
			_, _ = eb.Subscribe(tc.subject, func(_ context.Context, _ *bus.Event) error {
				handlerCalled++
				return nil
			})

			data := map[string]interface{}{
				"workspace_id": "ws-123",
				"task_id":      "t-456",
			}
			evt := bus.NewEvent(tc.subject, "test", data)
			if err := eb.Publish(context.Background(), tc.subject, evt); err != nil {
				t.Fatalf("Publish failed: %v", err)
			}

			// Our counter should have been called exactly once.
			if handlerCalled != 1 {
				t.Errorf("handler called %d times, want 1", handlerCalled)
			}
		})
	}
}

// TestStripPayloadKeys verifies the office re-broadcast payload transform:
// office.task.moved is workspace-scoped, so the session-scoped session_id it
// inherits from the source task.moved event must be dropped (otherwise the FE
// WS-account stamps the envelope session-routed and the bridge audit then
// expects a per-session cache mutation that no office handler makes on a
// non-office page). workspace_id and the rest must survive, and the source
// payload (shared with other subscribers) must not be mutated.
func TestStripPayloadKeys(t *testing.T) {
	source := map[string]interface{}{
		"workspace_id": "ws-1",
		"task_id":      "t-1",
		"session_id":   "sess-1",
		"to_step_id":   "step-2",
	}

	out := stripPayloadKeys(source, []string{"session_id"})
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map output, got %T", out)
	}
	if _, present := m["session_id"]; present {
		t.Error("session_id must be stripped from the office.task.moved payload")
	}
	if m["workspace_id"] != "ws-1" || m["task_id"] != "t-1" || m["to_step_id"] != "step-2" {
		t.Errorf("non-dropped fields must be preserved, got %#v", m)
	}
	// Source must be untouched — every other subscriber shares this map.
	if source["session_id"] != "sess-1" {
		t.Error("source payload must not be mutated")
	}
}

func TestStripPayloadKeys_NoKeysOrNonMap(t *testing.T) {
	source := map[string]interface{}{"a": 1}
	if got := stripPayloadKeys(source, nil); got == nil {
		t.Fatal("nil dropKeys should return the payload unchanged")
	}
	if got := stripPayloadKeys("not-a-map", []string{"x"}); got != "not-a-map" {
		t.Errorf("non-map payload should pass through, got %v", got)
	}
}

// TestOfficeEventBroadcaster_NilEventBus verifies no panic when event bus is nil.
func TestOfficeEventBroadcaster_NilEventBus(t *testing.T) {
	log := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterOfficeNotifications(ctx, nil, hub, nil, log)
	if len(b.subscriptions) != 0 {
		t.Errorf("expected 0 subscriptions with nil event bus, got %d", len(b.subscriptions))
	}
}

// TestOfficeEventBroadcaster_Close verifies that Close unsubscribes all subscriptions.
func TestOfficeEventBroadcaster_Close(t *testing.T) {
	log := testLogger()
	eb := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterOfficeNotifications(ctx, eb, hub, nil, log)
	if len(b.subscriptions) == 0 {
		t.Fatal("expected subscriptions before close")
	}

	b.Close()
	if len(b.subscriptions) != 0 {
		t.Errorf("expected 0 subscriptions after Close, got %d", len(b.subscriptions))
	}
}

// TestOfficeEventBroadcaster_WorkspaceForEvent covers workspace attribution:
// an explicit workspace_id wins; otherwise the task_id is resolved to its
// workspace; and an unresolvable event yields "" (caller fails closed).
func TestOfficeEventBroadcaster_WorkspaceForEvent(t *testing.T) {
	b := &OfficeEventBroadcaster{
		logger: testLogger(),
		resolveTaskWS: func(_ context.Context, taskID string) (string, error) {
			if taskID == "t-known" {
				return "ws-from-task", nil
			}
			return "", errors.New("unknown task")
		},
	}
	ctx := context.Background()

	cases := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{"explicit workspace_id wins", map[string]interface{}{"workspace_id": "ws-1", "task_id": "t-known"}, "ws-1"},
		{"resolved from task_id", map[string]interface{}{"task_id": "t-known"}, "ws-from-task"},
		{"unresolvable task", map[string]interface{}{"task_id": "t-missing"}, ""},
		{"no attribution", map[string]interface{}{"run_id": "r-1"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.workspaceForEvent(ctx, tc.data); got != tc.want {
				t.Errorf("workspaceForEvent = %q, want %q", got, tc.want)
			}
		})
	}

	// With a nil resolver, a task-only payload cannot be attributed.
	b.resolveTaskWS = nil
	if got := b.workspaceForEvent(ctx, map[string]interface{}{"task_id": "t-known"}); got != "" {
		t.Errorf("workspaceForEvent with nil resolver = %q, want empty", got)
	}
}

// TestOfficeRunEventRoutesByTaskWorkspaceAndFailsClosed proves the P1 fix:
// office run events carry task_id (no workspace_id), so under auth they must
// route to the owning workspace resolved from the task — and drop entirely when
// the workspace can't be resolved, never falling back to a global broadcast.
func TestOfficeRunEventRoutesByTaskWorkspaceAndFailsClosed(t *testing.T) {
	log := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := newAccessTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			if workspaceID == "ws-a" {
				return "user-a", nil
			}
			return "", errors.New("unknown workspace")
		},
	})
	resolver := func(_ context.Context, taskID string) (string, error) {
		if taskID == "t-known" {
			return "ws-a", nil
		}
		return "", errors.New("unknown task")
	}

	eb := bus.NewMemoryEventBus(log)
	_ = RegisterOfficeNotifications(ctx, eb, hub, resolver, log)

	clientA := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	clientB := registerAccessClient(t, hub, "b", authn.Identity{UserID: "user-b", Role: authn.RoleMember})

	publishRun := func(taskID string) {
		data := map[string]interface{}{"run_id": "r-1", "status": "queued", "task_id": taskID}
		evt := bus.NewEvent(events.OfficeRunProcessed, "test", data)
		if err := eb.Publish(context.Background(), events.OfficeRunProcessed, evt); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	// Owner routing: resolvable task → only the owning user receives it.
	// Owner-routed delivery is synchronous (sendToClients), so we can assert now.
	publishRun("t-known")
	if got := receivedActions(clientA); len(got) != 1 || got[0] != ws.ActionOfficeRunProcessed {
		t.Fatalf("owner received %v, want one %s", got, ws.ActionOfficeRunProcessed)
	}
	if got := receivedActions(clientB); len(got) != 0 {
		t.Fatalf("foreign user received %v, want none — WS LEAK", got)
	}

	// Fail closed: unresolvable task → dropped, NOT globally broadcast.
	publishRun("t-unknown")
	if got := receivedActions(clientA); len(got) != 0 {
		t.Fatalf("owner received %v for unresolvable run, want none (dropped)", got)
	}
	if got := receivedActions(clientB); len(got) != 0 {
		t.Fatalf("foreign user received %v for unresolvable run — WS LEAK", got)
	}
}
