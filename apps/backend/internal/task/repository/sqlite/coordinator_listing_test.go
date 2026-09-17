package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoordinatorListingExcludesHiddenTasksBeforePaging(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()
	for _, id := range []string{"hidden", "office"} {
		require.NoError(t, f.repo.CreateWorkflow(ctx, &models.Workflow{ID: id, WorkspaceID: autoOriginWorkspaceID, Name: id, Hidden: id == "hidden"}))
		require.NoError(t, f.repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: autoOriginWorkspaceID, WorkflowID: id, Title: "Example hidden task", State: "CREATED", Priority: "medium"}))
	}
	_, err := f.repo.DB().ExecContext(ctx, "UPDATE workspaces SET office_workflow_id = ? WHERE id = ?", "office", autoOriginWorkspaceID)
	require.NoError(t, err)
	require.NoError(t, f.repo.CreateTask(ctx, &models.Task{ID: "config", WorkspaceID: autoOriginWorkspaceID, WorkflowID: autoOriginWorkflowID, Title: "Example configuration", State: "CREATED", Priority: "medium", Metadata: map[string]interface{}{"config_mode": true}}))
	for _, query := range []string{"", "Example"} {
		rows, total, err := f.repo.ListKanbanTasksByWorkspace(ctx, autoOriginWorkspaceID, models.KanbanTaskQuery{Query: query, Page: 1, PageSize: 1})
		require.NoError(t, err)
		if query == "" {
			require.Equal(t, 1, total)
			require.Len(t, rows, 1)
			require.Equal(t, f.boardTaskID, rows[0].ID)
		} else {
			require.Zero(t, total)
			require.Empty(t, rows)
		}
	}
}
