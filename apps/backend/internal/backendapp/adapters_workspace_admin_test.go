package backendapp

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/workflow/stepevents"
	"github.com/stretchr/testify/require"
)

type workspaceStepRecorder struct{ subjects []string }

func (r *workspaceStepRecorder) Publish(_ context.Context, subject string, _ *bus.Event) error {
	r.subjects = append(r.subjects, subject)
	return nil
}

func newWorkspaceAdminHarness(t *testing.T) (*workspaceAdminAdapter, *workspaceStepRecorder) {
	t.Helper()
	a, svc := newOfficeTaskAdapterHarness(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	workflows := workflowservice.NewService(a.workflow, log)
	t.Cleanup(func() { require.NoError(t, workflows.Close()) })
	provider := &workflowProviderAdapter{svc: svc}
	workflows.SetWorkflowProvider(provider)
	workflows.SetWorkspaceProvider(provider)
	svc.SetWorkflowStepCreator(workflows)
	recorder := &workspaceStepRecorder{}
	return &workspaceAdminAdapter{taskCreatorAdapter: a, workflows: workflows, stepEvents: stepevents.NewPublisher(recorder, "test", log)}, recorder
}

func adminCommand(t *testing.T, a *workspaceAdminAdapter, resource, action, id string, config map[string]any) any {
	t.Helper()
	body, err := json.Marshal(map[string]any{"resource": resource, "action": action, "id": id, "configuration": config})
	require.NoError(t, err)
	result, err := a.ManageWorkspace(context.Background(), "ws-1", body)
	require.NoError(t, err)
	return result
}

func TestWorkspaceAdministrationNativeLifecycle(t *testing.T) {
	a, events := newWorkspaceAdminHarness(t)
	ctx := context.Background()
	adminCommand(t, a, "workspace", "update", "", map[string]any{"name": "Synthetic workspace", "description": "Example"})
	ws, err := a.taskSvc.GetWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.Equal(t, "Synthetic workspace", ws.Name)
	wf := adminCommand(t, a, "workflow", "create", "", map[string]any{"name": "Synthetic delivery"}).(*models.Workflow)
	adminCommand(t, a, "workflow", "update", wf.ID, map[string]any{"name": "Updated delivery", "prompt": "Use generic examples"})
	first := adminCommand(t, a, "step", "create", "", map[string]any{"workflow_id": wf.ID, "name": "Backlog", "is_start_step": true, "allow_manual_move": true}).(*wfmodels.WorkflowStep)
	second := adminCommand(t, a, "step", "create", "", map[string]any{"workflow_id": wf.ID, "name": "Doing", "position": 1, "is_start_step": true, "allow_manual_move": true}).(*wfmodels.WorkflowStep)
	adminCommand(t, a, "step", "update", second.ID, map[string]any{"name": "In progress", "wip_limit": 2})
	got, err := a.workflow.GetStep(ctx, first.ID)
	require.NoError(t, err)
	require.False(t, got.IsStartStep)
	got, err = a.workflow.GetStep(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, 2, got.WIPLimit)
	require.Equal(t, []string{"workflow_step.created", "workflow_step.updated", "workflow_step.created", "workflow_step.updated"}, events.subjects)
	adminCommand(t, a, "step", "delete", second.ID, nil)
	_, err = a.workflow.GetStep(ctx, second.ID)
	require.Error(t, err)
	adminCommand(t, a, "workflow", "delete", wf.ID, nil)
	_, err = a.taskSvc.GetWorkflow(ctx, wf.ID)
	require.Error(t, err)
}

func TestWorkspaceAdministrationRepositoryRegistration(t *testing.T) {
	a, _ := newWorkspaceAdminHarness(t)
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	repo := adminCommand(t, a, "repository", "create", "", map[string]any{"name": "Sample repository", "local_path": dir, "source_type": "local", "default_branch": "main"}).(*models.Repository)
	require.Equal(t, "ws-1", repo.WorkspaceID)
	adminCommand(t, a, "repository", "update", repo.ID, map[string]any{"name": "Updated repository"})
	got, err := a.taskSvc.GetRepository(context.Background(), repo.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated repository", got.Name)
	adminCommand(t, a, "repository", "delete", repo.ID, nil)
	_, err = a.taskSvc.GetRepository(context.Background(), repo.ID)
	require.Error(t, err)
}

func TestWorkspaceAdministrationRejectsForeignAndSystemResources(t *testing.T) {
	a, _ := newWorkspaceAdminHarness(t)
	ctx := context.Background()
	require.NoError(t, a.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "other", Name: "Other"}))
	wf, err := a.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "other", Name: "Other workflow"})
	require.NoError(t, err)
	require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "foreign-step", WorkflowID: wf.ID, Name: "Foreign"}))
	for _, body := range []string{
		`{"resource":"workspace","action":"update","id":"other","configuration":{"name":"Changed"}}`,
		`{"resource":"workspace","action":"update","configuration":{"unit_id":"other"}}`,
		`{"resource":"workflow","action":"create","configuration":{"name":"Changed","workspace_id":"other"}}`,
		`{"resource":"workflow","action":"create","configuration":{"name":"Hidden","hidden":true}}`,
		`{"resource":"step","action":"delete","id":"foreign-step"}`,
		`{"resource":"step","action":"update","id":"foreign-step","configuration":{"name":"Changed"}}`,
		`{"resource":"workspace","action":"delete"}`,
	} {
		_, err = a.ManageWorkspace(ctx, "ws-1", json.RawMessage(body))
		require.Error(t, err, body)
	}
	_, err = a.ManageWorkspace(ctx, "ws-1", json.RawMessage(`{"resource":"workflow","action":"delete","id":"`+wf.ID+`"}`))
	require.Error(t, err)
	got, err := a.workflow.GetStep(ctx, "foreign-step")
	require.NoError(t, err)
	require.Equal(t, "Foreign", got.Name)
}
