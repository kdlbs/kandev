package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdministrationRequiresNativeManagementPermission(t *testing.T) {
	a, _ := newWorkspaceAdminHarness(t)
	ctx := context.Background()
	ws, err := a.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: "Owned workspace", OwnerID: "owner"})
	require.NoError(t, err)
	wf, err := a.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: ws.ID, Name: "Original"})
	require.NoError(t, err)
	require.NoError(t, a.taskRepo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{WorkspaceID: ws.ID, UserID: "viewer", Role: "viewer"}))
	viewer := authn.WithIdentity(ctx, authn.Identity{UserID: "viewer", Role: authn.RoleMember})
	_, err = a.taskSvc.GetWorkspace(viewer, ws.ID)
	require.NoError(t, err, "fixture must grant read access")
	raw, err := json.Marshal(map[string]any{"resource": "workflow", "action": "update", "id": wf.ID, "configuration": map[string]string{"name": "Changed"}})
	require.NoError(t, err)
	_, err = a.ManageWorkspace(viewer, ws.ID, raw)
	require.Error(t, err)
	got, err := a.taskSvc.GetWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Equal(t, "Original", got.Name)
}

func TestWorkspaceAdministrationNativeWorkflowGuards(t *testing.T) {
	a, _ := newWorkspaceAdminHarness(t)
	ctx := context.Background()
	wf := adminCommand(t, a, "workflow", "create", "", map[string]any{"name": "Delivery"}).(*models.Workflow)
	first := adminCommand(t, a, "step", "create", "", map[string]any{"workflow_id": wf.ID, "name": "First"}).(*wfmodels.WorkflowStep)
	second := adminCommand(t, a, "step", "create", "", map[string]any{"workflow_id": wf.ID, "name": "Second", "position": 1}).(*wfmodels.WorkflowStep)
	command := func(body any) error {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		_, err = a.ManageWorkspace(ctx, "ws-1", raw)
		return err
	}
	require.NoError(t, command(map[string]any{"resource": "step", "action": "reorder", "workflow_id": wf.ID, "ids": []string{second.ID, first.ID}}))
	rows, err := a.workflow.ListStepsByWorkflow(ctx, wf.ID)
	require.NoError(t, err)
	require.Equal(t, second.ID, rows[0].ID)
	for _, config := range []map[string]any{
		{"wip_limit": -1},
		{"pull_from_step_id": first.ID},
		{"events": map[string]any{"on_enter": []any{map[string]any{"type": "queue_run", "config": map[string]string{"task_id": "foreign-task"}}}}},
	} {
		require.Error(t, command(map[string]any{"resource": "step", "action": "update", "id": first.ID, "configuration": config}))
	}
	require.NoError(t, a.taskSvc.SetWorkflowSource(ctx, wf.ID, models.WorkflowSourceGitHub, "example.yaml"))
	for _, resource := range []string{"workflow", "step"} {
		id := wf.ID
		if resource == "step" {
			id = first.ID
		}
		for _, action := range []string{"update", "delete"} {
			require.ErrorContains(t, command(map[string]any{"resource": resource, "action": action, "id": id, "configuration": map[string]string{"name": "Changed"}}), "read-only")
		}
	}
	got, err := a.workflow.GetStep(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "First", got.Name)
}

func TestWorkspaceAdministrationRejectsForeignReviewProfile(t *testing.T) {
	a, _ := newWorkspaceAdminHarness(t)
	a.profiles = workspaceTestProfiles{"foreign": &settingsmodels.AgentProfile{ID: "foreign", Enabled: true, WorkspaceID: "other"}}
	wf := adminCommand(t, a, "workflow", "create", "", map[string]any{"name": "Delivery"}).(*models.Workflow)
	raw, err := json.Marshal(map[string]any{"resource": "step", "action": "create", "configuration": map[string]any{
		"name": "Review", "workflow_id": wf.ID, "events": map[string]any{"on_enter": []any{map[string]any{"type": "run_code_review", "config": map[string]string{"agent_profile_id": "foreign"}}}},
	}})
	require.NoError(t, err)
	_, err = a.ManageWorkspace(context.Background(), "ws-1", raw)
	require.Error(t, err)
	steps, err := a.workflow.ListStepsByWorkflow(context.Background(), wf.ID)
	require.NoError(t, err)
	require.Empty(t, steps)
}

func TestWorkspaceAdministrationForeignReferencesDoNotMutate(t *testing.T) {
	a, _ := newWorkspaceAdminHarness(t)
	ctx := context.Background()
	require.NoError(t, a.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "other", Name: "Other"}))
	foreign, err := a.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "other", Name: "Foreign"})
	require.NoError(t, err)
	require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "foreign-step", WorkflowID: foreign.ID, Name: "Foreign"}))
	require.NoError(t, a.taskRepo.CreateTask(ctx, &models.Task{ID: "foreign-task", WorkspaceID: "other", WorkflowID: foreign.ID, Title: "Foreign"}))
	wf := adminCommand(t, a, "workflow", "create", "", map[string]any{"name": "Delivery"}).(*models.Workflow)
	step := adminCommand(t, a, "step", "create", "", map[string]any{"workflow_id": wf.ID, "name": "Original"}).(*wfmodels.WorkflowStep)
	for _, configuration := range []map[string]any{
		{"name": "Changed", "pull_from_step_id": "foreign-step"},
		{"name": "Changed", "events": map[string]any{"on_turn_complete": []any{map[string]any{"type": "move_to_step", "config": map[string]string{"step_id": "foreign-step"}}}}},
		{"name": "Changed", "events": map[string]any{"on_enter": []any{map[string]any{"type": "queue_run", "config": map[string]string{"task_id": "foreign-task"}}}}},
	} {
		raw, err := json.Marshal(map[string]any{"resource": "step", "action": "update", "id": step.ID, "configuration": configuration})
		require.NoError(t, err)
		_, err = a.ManageWorkspace(ctx, "ws-1", raw)
		require.Error(t, err)
	}
	got, err := a.workflow.GetStep(ctx, step.ID)
	require.NoError(t, err)
	require.Equal(t, "Original", got.Name)
}
