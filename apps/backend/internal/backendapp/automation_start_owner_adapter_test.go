package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestReviewTaskCreatorAdapterPreservesInitialStartOwner(t *testing.T) {
	svc, repo := newTaskChangeCoordinatorHarness(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "owner-workspace", Name: "Owner"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "owner-workflow", WorkspaceID: "owner-workspace", Name: "Owner"}))
	adapter := &reviewTaskCreatorAdapter{svc: svc}
	task, err := adapter.CreateReviewTask(ctx, &orchestrator.ReviewTaskRequest{
		WorkspaceID: "owner-workspace", WorkflowID: "owner-workflow", Title: "Automation",
		Origin: models.TaskOriginAutomationTask, AutomationStartOwned: true, AutomationStartOwner: "admitted-run",
	})
	require.NoError(t, err)
	require.Equal(t, "admitted-run", task.Metadata[models.MetaKeyAutomationStartOwned])
	persisted, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "admitted-run", persisted.Metadata[models.MetaKeyAutomationStartOwned])
}
