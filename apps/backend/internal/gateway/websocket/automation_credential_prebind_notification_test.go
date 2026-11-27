package websocket

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.4
func TestProtectedAutomationTaskCreatedIsRedactedBeforeRunBinding(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	store, err := automation.NewStore(database, database)
	require.NoError(t, err)

	creatorContext := authn.WithIdentity(context.Background(), authn.Identity{UserID: "automation-creator", Role: authn.RoleMember})
	definition := &automation.Automation{WorkspaceID: "prebind-workspace", Name: "protected automation"}
	require.NoError(t, store.CreateAutomationWithCredentialChoices(creatorContext, definition, []automation.AutomationCredentialChoice{{
		Kind: automation.CredentialChoicePersonal, ProfileID: "profile-1", Key: "SERVICE_KEY",
		SecretID: "personal-secret", SourceSecretID: "profile-secret",
	}}))
	run := &automation.AutomationRun{AutomationID: definition.ID, TriggerType: automation.TriggerTypeManual, Status: automation.RunStatusTriggered}
	require.NoError(t, store.CreateRun(creatorContext, run))
	creatorForTask, err := store.ProtectedCreatorForTask(context.Background(), "prebind-task")
	require.NoError(t, err)
	require.Empty(t, creatorForTask, "the task has not been linked to its automation run yet")

	hub := newTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		WorkspaceReaders: func(_ context.Context, workspaceID string) ([]string, error) {
			require.Equal(t, definition.WorkspaceID, workspaceID)
			return []string{"workspace-reader"}, nil
		},
	})
	reader := newAuthorizedNotificationClient(hub, "reader", "workspace-reader")
	broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}
	broadcaster.SetProtectedTaskLookup(func(ctx context.Context, taskID string) (bool, error) {
		creator, lookupErr := store.ProtectedCreatorForTask(ctx, taskID)
		return creator != "", lookupErr
	})
	broadcaster.SetProtectedAutomationRunLookup(func(ctx context.Context, runID string) (bool, error) {
		linkedRun, lookupErr := store.GetRun(ctx, runID)
		return linkedRun != nil && linkedRun.ProtectedCreatorUserID != "", lookupErr
	})
	data := map[string]interface{}{
		"task_id": "prebind-task", "workspace_id": definition.WorkspaceID,
		"title": "private title", "description": "private description", "session_id": "private-session",
		"metadata": map[string]interface{}{"automation_run_id": run.ID},
	}
	require.NoError(t, broadcaster.broadcastEvent(context.Background(), bus.NewEvent("task.created", "test", data), ws.ActionTaskCreated))

	frame := nextClientFrame(t, reader)
	require.Equal(t, ws.ActionTaskCreated, frame.Action)
	require.Equal(t, map[string]interface{}{"task_id": "prebind-task", "workspace_id": definition.WorkspaceID}, notificationPayload(t, frame))
	assertNoMoreFrames(t, reader)
}
