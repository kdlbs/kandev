package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskLifecycleReceiptsPersistCreateArchiveUnarchiveAndDelete(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "visibility-workspace")
	task := &models.Task{ID: "visibility-task", WorkspaceID: "visibility-workspace", WorkflowID: "workflow", WorkflowStepID: "step", Title: "Receipt target", Identifier: "KAN-1"}
	require.NoError(t, repo.CreateTask(ctx, task))

	receipt, err := repo.GetTaskLifecycleReceipt(ctx, "visibility-workspace", task.ID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, "active", receipt.Disposition)
	require.Equal(t, "create", receipt.Provenance)
	require.EqualValues(t, 1, receipt.Revision)

	require.NoError(t, repo.ArchiveTask(ctx, task.ID))
	receipt, err = repo.GetTaskLifecycleReceipt(ctx, "visibility-workspace", task.ID)
	require.NoError(t, err)
	require.Equal(t, "archived", receipt.Disposition)
	require.Equal(t, "archive", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)

	changed, err := repo.UnarchiveTask(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, changed)
	receipt, err = repo.GetTaskLifecycleReceipt(ctx, "visibility-workspace", task.ID)
	require.NoError(t, err)
	require.Equal(t, "active", receipt.Disposition)
	require.Equal(t, "unarchive", receipt.Provenance)
	require.EqualValues(t, 3, receipt.Revision)

	require.NoError(t, repo.DeleteTask(ctx, task.ID))
	receipt, err = repo.GetTaskLifecycleReceipt(ctx, "visibility-workspace", task.ID)
	require.NoError(t, err)
	require.Equal(t, "deleted", receipt.Disposition)
	require.Equal(t, "delete", receipt.Provenance)
	require.EqualValues(t, 4, receipt.Revision)
	foreign, err := repo.GetTaskLifecycleReceipt(ctx, "other-workspace", task.ID)
	require.NoError(t, err)
	require.Nil(t, foreign)
}

func TestCreateTaskRollsBackWhenLifecycleReceiptWriteFails(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "receipt-rollback-workspace")
	_, err := repo.db.Exec(`DROP TABLE task_lifecycle_receipts`)
	require.NoError(t, err)

	err = repo.CreateTask(ctx, &models.Task{ID: "receipt-rollback-task", WorkspaceID: "receipt-rollback-workspace", Title: "must not persist"})
	require.Error(t, err)
	_, err = repo.GetTask(ctx, "receipt-rollback-task")
	require.Error(t, err, "the task row must roll back with its required receipt")
}
