package backendapp

import (
	"context"
	"testing"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMaintenanceBoundaryNativeLaunch(t *testing.T) {
	guard := assistantDispatchGuard(nil, nil)
	task := &models.Task{ID: "repair", Metadata: map[string]any{"orchestration_maintenance_candidate": "candidate"}}
	require.Error(t, guard(context.Background(), task, nil, "profile"), "maintenance tasks use the closed broker even when Assistant execution is disabled")
}

func TestAssistantMaintenanceScopeNativeResources(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Review local repairs"})
	require.NoError(t, err)
	require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "review", WorkflowID: wf.ID, Name: "Review", IsStartStep: true}))
	dir := t.TempDir()
	require.NoError(t, a.taskRepo.CreateRepository(ctx, &models.Repository{ID: "repair-repo", WorkspaceID: "ws-1", Name: "Sample", LocalPath: dir}))
	scope := shared.MaintenanceScope{RepositoryID: "repair-repo", WorkflowID: wf.ID, WorkflowStepID: "review", ProfileID: "personal"}
	path, err := a.ValidateMaintenanceScope(ctx, "ws-1", scope)
	require.NoError(t, err)
	require.Equal(t, dir, path)
	_, err = a.ValidateMaintenanceScope(ctx, "foreign", scope)
	require.Error(t, err)
	template := "improve-kandev"
	wf.WorkflowTemplateID = &template
	require.NoError(t, a.taskRepo.UpdateWorkflow(ctx, wf))
	_, err = a.ValidateMaintenanceScope(ctx, "ws-1", scope)
	require.Error(t, err, "a local grant cannot enter contribution destination preparation")
}
