package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotNotificationScopesAndDropsUnattributedEvents(t *testing.T) {
	for _, action := range []string{"execution_changed", "sandbox_approvals_changed"} {
		t.Run(action, func(t *testing.T) { testExecutionNotificationScope(t, action) })
	}
}

func testExecutionNotificationScope(t *testing.T, action string) {
	t.Helper()
	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceOwner: func(_ context.Context, id string) (string, error) {
			if id == "workspace" {
				return "owner", nil
			}
			return "", errors.New("unknown workspace")
		},
	})
	owner, foreign := newTestClient("owner"), newTestClient("foreign")
	owner.identity = authn.Identity{UserID: "owner", Role: authn.RoleMember}
	foreign.identity = authn.Identity{UserID: "foreign", Role: authn.RoleMember}
	registerTestClient(hub, owner)
	registerTestClient(hub, foreign)
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(eventBus.Close)
	broadcaster := RegisterTaskNotifications(t.Context(), eventBus, hub, testLogger())
	t.Cleanup(broadcaster.Close)
	payload := map[string]any{"workspace_id": "workspace", "task_id": "task", "session_id": "session", "queue_incarnation_id": "incarnation", "execution_snapshot": map[string]any{"attempt_id": "attempt"}}
	require.NoError(t, eventBus.Publish(t.Context(), "task_session."+action, bus.NewEvent("task_session."+action, "test", payload)))
	select {
	case raw := <-owner.send:
		var message ws.Message
		require.NoError(t, json.Unmarshal(raw, &message))
		require.Equal(t, "session."+action, message.Action)
	default:
		t.Fatal("execution update did not reach its workspace owner")
	}
	require.False(t, clientReceived(foreign))
	delete(payload, "workspace_id")
	require.NoError(t, broadcaster.broadcastEvent(t.Context(), bus.NewEvent("task_session."+action, "test", payload), "session."+action))
	require.False(t, clientReceived(owner), "unattributed execution update must be dropped")
	require.False(t, clientReceived(foreign))
}
