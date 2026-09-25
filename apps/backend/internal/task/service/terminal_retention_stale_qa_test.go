package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

type retentionBeforeOrdinaryTaskWrite struct {
	taskrepo.TaskRepository
	parentID    string
	workspaceID string
	held        bool
}

func (r retentionBeforeOrdinaryTaskWrite) UpdateTaskPreservingDeferredLaunch(ctx context.Context, task *models.Task) error {
	writer := r.TaskRepository.(interface {
		UpdateTaskTerminalRetentionIfParent(context.Context, string, string, string, bool) (bool, error)
	})
	changed, err := writer.UpdateTaskTerminalRetentionIfParent(ctx, task.ID, r.parentID, r.workspaceID, r.held)
	if err != nil {
		return err
	}
	if !changed {
		return taskrepo.ErrTaskNotFound
	}
	return r.TaskRepository.UpdateTaskPreservingDeferredLaunch(ctx, task)
}

func TestQAOrdinaryTaskEditPreservesConcurrentTerminalRetention(t *testing.T) {
	t.Run("concurrent hold", func(t *testing.T) {
		assertOrdinaryEditPreservesRetention(t, false, true)
	})
	t.Run("concurrent clear", func(t *testing.T) {
		assertOrdinaryEditPreservesRetention(t, true, false)
	})
}

func assertOrdinaryEditPreservesRetention(t *testing.T, initialHeld, concurrentHeld bool) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	for _, task := range []*models.Task{
		{ID: "retention-parent", WorkspaceID: workspace.ID, WorkflowID: workspace.OfficeWorkflowID, Title: "Parent"},
		{ID: "retention-child", WorkspaceID: workspace.ID, WorkflowID: workspace.OfficeWorkflowID, ParentID: "retention-parent", Title: "Child"},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	if initialHeld {
		_, err = repo.UpdateTaskTerminalRetentionIfParent(ctx, "retention-child", "retention-parent", workspace.ID, true)
		require.NoError(t, err)
	}
	svc.tasks = retentionBeforeOrdinaryTaskWrite{
		TaskRepository: repo, parentID: "retention-parent", workspaceID: workspace.ID, held: concurrentHeld,
	}
	title := "Edited child"
	_, err = svc.UpdateTask(ctx, "retention-child", &UpdateTaskRequest{Title: &title})
	require.NoError(t, err)
	stored, err := repo.GetTask(ctx, "retention-child")
	require.NoError(t, err)
	require.Equal(t, title, stored.Title)
	require.Equal(t, concurrentHeld, models.IsTerminalRetentionHeld(stored.Metadata), "ordinary edit overwrote concurrent retention update")
}
