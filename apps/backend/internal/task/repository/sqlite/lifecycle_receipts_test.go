package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskVisibilitySnapshotKeepsTaskAndReceiptCoherentAcrossDelete(t *testing.T) {
	ctx := context.Background()
	repo, writer := newVisibilitySnapshotRepo(t)
	seedWorkspace(t, repo, "visibility-snapshot-workspace")
	task := &models.Task{ID: "visibility-snapshot-task", WorkspaceID: "visibility-snapshot-workspace", Title: "Before delete", Identifier: "VIS-3"}
	require.NoError(t, repo.CreateTask(ctx, task))
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "visibility-snapshot-session", TaskID: task.ID, State: models.TaskSessionStateCompleted,
		StartedAt: now, CompletedAt: &now, UpdatedAt: now,
	}))
	_, err := writer.Exec(`
		INSERT INTO task_documents (id, task_id, key, type, title, content, author_kind, author_name, filename, mime_type, size_bytes, disk_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "visibility-snapshot-document", task.ID, "visibility", "plan", "Visibility", "content", "agent", "test", "", "", 0, "", now, now)
	require.NoError(t, err)
	_, err = writer.Exec(`
		INSERT INTO task_document_revisions (id, task_id, document_key, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
	`, "visibility-snapshot-document-revision", task.ID, "visibility", 1, "Visibility", "content", "agent", "test", now, now)
	require.NoError(t, err)
	tables := []string{"tasks", "task_lifecycle_receipts", "task_sessions", "task_documents", "task_document_revisions"}
	beforeRead := make(map[string][]string, len(tables))
	for _, table := range tables {
		beforeRead[table] = snapshotTableRows(t, writer, table)
	}
	readTaskOnly, readReceiptOnly, err := repo.GetTaskVisibilitySnapshot(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, readTaskOnly)
	require.NotNil(t, readReceiptOnly)
	for _, table := range tables {
		require.Equal(t, beforeRead[table], snapshotTableRows(t, writer, table), table)
	}

	readTask := make(chan struct{})
	continueRead := make(chan struct{})
	var continueOnce sync.Once
	releaseRead := func() { continueOnce.Do(func() { close(continueRead) }) }
	t.Cleanup(releaseRead)
	repo.visibilitySnapshotAfterTaskRead = func() {
		close(readTask)
		<-continueRead
	}
	type result struct {
		task    *models.Task
		receipt *models.TaskLifecycleReceipt
		err     error
	}
	resultCh := make(chan result, 1)
	go func() {
		task, receipt, err := repo.GetTaskVisibilitySnapshot(ctx, "visibility-snapshot-workspace", "visibility-snapshot-task")
		resultCh <- result{task: task, receipt: receipt, err: err}
	}()
	<-readTask
	require.NoError(t, repo.DeleteTask(ctx, task.ID))
	releaseRead()
	first := <-resultCh
	require.NoError(t, first.err)
	require.NotNil(t, first.task)
	require.NotNil(t, first.receipt)
	require.Equal(t, "Before delete", first.task.Title)
	require.Equal(t, "active", first.receipt.Disposition)
	require.Equal(t, "create", first.receipt.Provenance)
	require.EqualValues(t, 1, first.receipt.Revision)

	repo.visibilitySnapshotAfterTaskRead = nil
	nextTask, nextReceipt, err := repo.GetTaskVisibilitySnapshot(ctx, "visibility-snapshot-workspace", task.ID)
	require.NoError(t, err)
	require.Nil(t, nextTask)
	require.NotNil(t, nextReceipt)
	require.Equal(t, "deleted", nextReceipt.Disposition)
	require.Equal(t, "delete", nextReceipt.Provenance)
	require.EqualValues(t, 2, nextReceipt.Revision)

	second := &models.Task{ID: "visibility-split-mutant", WorkspaceID: "visibility-snapshot-workspace", Title: "Split target"}
	require.NoError(t, repo.CreateTask(ctx, second))
	oldTask, err := repo.GetTask(ctx, second.ID)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteTask(ctx, second.ID))
	newReceipt, err := repo.GetTaskLifecycleReceipt(ctx, second.WorkspaceID, second.ID)
	require.NoError(t, err)
	require.NotNil(t, oldTask)
	require.NotNil(t, newReceipt)
	require.Equal(t, "deleted", newReceipt.Disposition, "a split read mutant produces a torn task/receipt pair")
}

func newVisibilitySnapshotRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "visibility-snapshot.db")
	writerConn, err := db.OpenSQLite(path)
	require.NoError(t, err)
	readerConn, err := db.OpenSQLiteReader(path)
	require.NoError(t, err)
	writer := sqlx.NewDb(writerConn, "sqlite3")
	reader := sqlx.NewDb(readerConn, "sqlite3")
	repo, err := NewWithDB(writer, reader, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	return repo, writer
}

func snapshotTableRows(t *testing.T, db *sqlx.DB, table string) []string {
	t.Helper()
	rows, err := db.Queryx("SELECT * FROM " + table + " ORDER BY rowid")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var result []string
	for rows.Next() {
		values, err := rows.SliceScan()
		require.NoError(t, err)
		result = append(result, fmt.Sprint(values...))
	}
	require.NoError(t, rows.Err())
	return result
}

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

func TestLifecycleMutationsRollBackWhenReceiptWriteFails(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		prepare func(*Repository, *models.Task)
		mutate  func(*Repository, *models.Task) error
		assert  func(*testing.T, *models.Task)
	}{
		{
			name:   "archive",
			mutate: func(repo *Repository, task *models.Task) error { return repo.ArchiveTask(ctx, task.ID) },
			assert: func(t *testing.T, task *models.Task) {
				require.Nil(t, task.ArchivedAt)
			},
		},
		{
			name: "unarchive",
			prepare: func(repo *Repository, task *models.Task) {
				require.NoError(t, repo.ArchiveTask(ctx, task.ID))
			},
			mutate: func(repo *Repository, task *models.Task) error {
				_, err := repo.UnarchiveTask(ctx, task.ID)
				return err
			},
			assert: func(t *testing.T, task *models.Task) {
				require.NotNil(t, task.ArchivedAt)
			},
		},
		{
			name:   "delete",
			mutate: func(repo *Repository, task *models.Task) error { return repo.DeleteTask(ctx, task.ID) },
			assert: func(t *testing.T, task *models.Task) {
				require.Equal(t, "delete", task.Title)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			workspaceID := "receipt-rollback-" + tc.name
			seedWorkspace(t, repo, workspaceID)
			task := &models.Task{ID: "receipt-rollback-" + tc.name, WorkspaceID: workspaceID, Title: tc.name}
			require.NoError(t, repo.CreateTask(ctx, task))
			if tc.prepare != nil {
				tc.prepare(repo, task)
			}
			_, err := repo.db.Exec(`DROP TABLE task_lifecycle_receipts`)
			require.NoError(t, err)
			require.Error(t, tc.mutate(repo, task))
			stored, err := repo.GetTask(ctx, task.ID)
			require.NoError(t, err)
			tc.assert(t, stored)
		})
	}
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
