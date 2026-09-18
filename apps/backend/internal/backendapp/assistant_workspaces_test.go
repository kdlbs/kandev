package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/auth/authn"
	orchestrationmodels "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantWorkspaceNativeEffectGuard(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Example delivery"})
	require.NoError(t, err)
	denied := errors.New("workspace access revoked")
	checks := 0
	guarded := orchestrationmodels.WithWorkspaceEffectGuard(ctx, func(context.Context) error {
		checks++
		return denied
	})
	spec := orchestrationmodels.WorkspaceTaskSpec{WorkspaceID: "ws-1", WorkflowID: wf.ID, Title: "Example denied delivery"}
	_, err = a.CreateWorkspaceTask(guarded, spec)
	require.ErrorIs(t, err, denied)
	require.Equal(t, 1, checks)
	rows, _, err := a.AssistantWorkspaceTasks(ctx, "ws-1", 1, 10)
	require.NoError(t, err)
	require.Empty(t, rows, "revocation immediately before the native effect cannot create a task")
	_, err = a.CreateWorkspaceTask(ctx, spec)
	require.NoError(t, err, "the same native request is otherwise valid")
}

func TestAssistantWorkspaceNativeVisibility(t *testing.T) {
	a, _ := newOfficeTaskAdapterHarness(t)
	require.NoError(t, a.taskRepo.CreateWorkspace(context.Background(), &models.Workspace{ID: "owner-workspace", Name: "Example private workspace", OwnerID: "owner"}))
	owner := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	row, err := a.WorkspaceGrantAccess(owner, "owner-workspace", false)
	require.NoError(t, err)
	require.Equal(t, "Example private workspace", row.Name)
	_, err = a.WorkspaceGrantAccess(owner, "missing", false)
	require.Error(t, err)
	foreign := authn.WithIdentity(context.Background(), authn.Identity{UserID: "foreign", Role: authn.RoleMember})
	_, err = a.WorkspaceGrantAccess(foreign, "owner-workspace", true)
	require.Error(t, err)
	rows, err := a.WorkspaceGrantOptions(foreign)
	require.NoError(t, err)
	for _, choice := range rows {
		require.NotEqual(t, "owner-workspace", choice.ID)
	}
}

func TestAssistantWorkspaceNativeProjection(t *testing.T) {
	a, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	require.NoError(t, a.taskRepo.CreateTask(ctx, &models.Task{ID: "linked-task", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Example delivery", Description: "SYNTHETIC_PRIVATE_TASK_PROMPT", Metadata: map[string]any{"private": "SYNTHETIC_PRIVATE_METADATA"}}))
	view, err := a.AssistantWorkspaceTask(ctx, "ws-1", "linked-task", false)
	require.NoError(t, err)
	require.Equal(t, "Example delivery", view.Task.Title)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "SYNTHETIC_PRIVATE")
	_, err = a.AssistantWorkspaceTask(ctx, "foreign", "linked-task", false)
	require.Error(t, err)
	rows, _, err := a.AssistantWorkspaceTasks(ctx, "ws-1", 1, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "linked-task", rows[0].ID)
}
