package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
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

func TestTaskLifecycleReceiptSurvivesRepositoryRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "receipt-restart.db")
	conn, err := db.OpenSQLite(path)
	require.NoError(t, err)
	firstDB := sqlx.NewDb(conn, "sqlite3")
	first, err := NewWithDB(firstDB, firstDB, nil)
	require.NoError(t, err)
	seedWorkspace(t, first, "receipt-restart-workspace")
	require.NoError(t, first.CreateTask(ctx, &models.Task{ID: "receipt-restart-task", WorkspaceID: "receipt-restart-workspace", Title: "persisted"}))
	require.NoError(t, firstDB.Close())

	conn, err = db.OpenSQLite(path)
	require.NoError(t, err)
	secondDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = secondDB.Close() })
	second, err := NewWithDB(secondDB, secondDB, nil)
	require.NoError(t, err)
	receipt, err := second.GetTaskLifecycleReceipt(ctx, "receipt-restart-workspace", "receipt-restart-task")
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, "create", receipt.Provenance)
	require.EqualValues(t, 1, receipt.Revision)
}

func TestDeletedTaskLifecycleReceiptSurvivesRepositoryRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "deleted-receipt-restart.db")
	conn, err := db.OpenSQLite(path)
	require.NoError(t, err)
	firstDB := sqlx.NewDb(conn, "sqlite3")
	first, err := NewWithDB(firstDB, firstDB, nil)
	require.NoError(t, err)
	seedWorkspace(t, first, "deleted-restart-workspace")
	task := &models.Task{ID: "deleted-restart-task", WorkspaceID: "deleted-restart-workspace", Title: "deleted"}
	require.NoError(t, first.CreateTask(ctx, task))
	require.NoError(t, first.DeleteTask(ctx, task.ID))
	require.NoError(t, firstDB.Close())

	conn, err = db.OpenSQLite(path)
	require.NoError(t, err)
	secondDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = secondDB.Close() })
	second, err := NewWithDB(secondDB, secondDB, nil)
	require.NoError(t, err)
	receipt, err := second.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, "deleted", receipt.Disposition)
	require.Equal(t, "delete", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)
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

func TestGetTaskLifecycleReceiptDoesNotMutateStoredReceipt(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "receipt-read-workspace")
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "receipt-read-task", WorkspaceID: "receipt-read-workspace", Title: "read-only"}))

	var beforeRevision int64
	var beforeChangedAt string
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT revision, changed_at FROM task_lifecycle_receipts WHERE task_id = ?`, "receipt-read-task").Scan(&beforeRevision, &beforeChangedAt))
	for range 2 {
		receipt, err := repo.GetTaskLifecycleReceipt(ctx, "receipt-read-workspace", "receipt-read-task")
		require.NoError(t, err)
		require.NotNil(t, receipt)
	}
	var afterRevision int64
	var afterChangedAt string
	require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT revision, changed_at FROM task_lifecycle_receipts WHERE task_id = ?`, "receipt-read-task").Scan(&afterRevision, &afterChangedAt))
	require.Equal(t, beforeRevision, afterRevision)
	require.Equal(t, beforeChangedAt, afterChangedAt)
}

func TestTaskLifecycleReceiptTracksWorkflowAttachAndQuickChatExpiry(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "receipt-move-workspace")
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "receipt-move-workflow", WorkspaceID: "receipt-move-workspace", Name: "Move"}))
	_, err := repo.db.ExecContext(ctx, `INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`, "receipt-move-step", "receipt-move-workflow", "Work")
	require.NoError(t, err)

	move := &models.Task{ID: "receipt-move-task", WorkspaceID: "receipt-move-workspace", Title: "Move"}
	require.NoError(t, repo.CreateTask(ctx, move))
	require.NoError(t, repo.AddTaskToWorkflow(ctx, move.ID, "receipt-move-workflow", "receipt-move-step", 0))
	receipt, err := repo.GetTaskLifecycleReceipt(ctx, move.WorkspaceID, move.ID)
	require.NoError(t, err)
	require.Equal(t, "move", receipt.Provenance)
	require.Equal(t, "receipt-move-workflow", receipt.WorkflowID)
	require.EqualValues(t, 2, receipt.Revision)

	expiring := &models.Task{ID: "receipt-expiry-task", WorkspaceID: "receipt-move-workspace", Title: "Expiry", IsEphemeral: true}
	require.NoError(t, repo.CreateTask(ctx, expiring))
	cutoff := time.Now().UTC()
	_, err = repo.db.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, cutoff.Add(-time.Hour), expiring.ID)
	require.NoError(t, err)
	deleted, err := repo.DeleteExpiredQuickChatTask(ctx, expiring.ID, cutoff)
	require.NoError(t, err)
	require.True(t, deleted)
	receipt, err = repo.GetTaskLifecycleReceipt(ctx, expiring.WorkspaceID, expiring.ID)
	require.NoError(t, err)
	require.Equal(t, "deleted", receipt.Disposition)
	require.Equal(t, "delete", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)
}

func TestArchiveLifecycleReceiptDoesNotAdvanceOnNoOp(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "receipt-noop-workspace")
	task := &models.Task{ID: "receipt-noop-task", WorkspaceID: "receipt-noop-workspace", Title: "No-op"}
	require.NoError(t, repo.CreateTask(ctx, task))
	changed, err := repo.ArchiveTaskIfActive(ctx, task.ID, "cascade-noop")
	require.NoError(t, err)
	require.True(t, changed)
	first, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	changed, err = repo.ArchiveTaskIfActive(ctx, task.ID, "cascade-noop")
	require.NoError(t, err)
	require.False(t, changed)
	second, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.Equal(t, first.Revision, second.Revision)
	require.Equal(t, first.ChangedAt, second.ChangedAt)
}
