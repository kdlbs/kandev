package automation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestAutomationCredentialRunAccessProjection(t *testing.T) {
	svc := newTestService(t)
	a := &Automation{WorkspaceID: "credential-run-workspace", Name: "credential run"}
	ctx := credentialIdentity("credential-creator")
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	storePersonalCredentialChoice(t, svc.store, a)
	run := &AutomationRun{
		AutomationID: a.ID, TaskID: "credential-run-task",
		Status: RunStatusFailed, ErrorMessage: "provider returned secret-id=private", DisplayTitle: "private run title",
		TriggerData: json.RawMessage(`{"payload":"private"}`),
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	svc.SetWorkspaceCredentialGrantDependencies(func(context.Context, string) (string, error) {
		return "credential-owner", nil
	}, nil, nil, nil, nil)

	protected, allowed, err := svc.ProtectedAutomationRunAccessForTask(ctx, run.TaskID, "credential-viewer")
	require.NoError(t, err)
	require.True(t, protected)
	require.False(t, allowed)
	protected, allowed, err = svc.ProtectedAutomationRunAccessForTask(ctx, run.TaskID, "credential-creator")
	require.NoError(t, err)
	require.True(t, protected)
	require.True(t, allowed)

	viewer := authn.WithIdentity(ctx, authn.Identity{UserID: "credential-viewer", Role: authn.RoleMember})
	rows, err := svc.ListRuns(viewer, a.ID, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].TaskID)
	require.Empty(t, rows[0].DisplayTitle)
	require.Empty(t, rows[0].TriggerData)
	require.NotContains(t, rows[0].ErrorMessage, "secret-id")
	require.NotContains(t, rows[0].ErrorMessage, "private")
	require.False(t, rows[0].CanStop)

	encoded, err := json.Marshal(rows[0])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "protected_creator_user_id")
	creator := authn.WithIdentity(ctx, authn.Identity{UserID: "credential-creator", Role: authn.RoleMember})
	creatorRows, err := svc.ListRuns(creator, a.ID, 10)
	require.NoError(t, err)
	require.Equal(t, run.TaskID, creatorRows[0].TaskID)
	require.Equal(t, run.DisplayTitle, creatorRows[0].DisplayTitle)
	require.True(t, creatorRows[0].CanStop)
	owner := authn.WithIdentity(ctx, authn.Identity{UserID: "credential-owner", Role: authn.RoleMember})
	ownerRows, err := svc.ListRuns(owner, a.ID, 10)
	require.NoError(t, err)
	require.True(t, ownerRows[0].CanStop)
}
