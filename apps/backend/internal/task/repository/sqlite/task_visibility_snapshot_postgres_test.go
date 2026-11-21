package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresTaskVisibilitySnapshotKeepsTaskAndReceiptCoherentAcrossDelete(t *testing.T) {
	ctx := context.Background()
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	_, err = messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)

	workspaceID := "postgres-visibility-snapshot-workspace"
	task := &models.Task{
		ID:          "postgres-visibility-snapshot-task",
		WorkspaceID: workspaceID,
		Title:       "Before delete",
		Identifier:  "PGV-1",
	}
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Postgres visibility snapshot"}))
	require.NoError(t, repo.CreateTask(ctx, task))

	readTask := make(chan struct{})
	continueRead := make(chan struct{})
	var releaseOnce sync.Once
	releaseRead := func() { releaseOnce.Do(func() { close(continueRead) }) }
	t.Cleanup(releaseRead)
	repo.visibilitySnapshotAfterTaskRead = func() {
		close(readTask)
		<-continueRead
	}
	t.Cleanup(func() { repo.visibilitySnapshotAfterTaskRead = nil })

	type result struct {
		task    *models.Task
		receipt *models.TaskLifecycleReceipt
		err     error
	}
	resultCh := make(chan result, 1)
	go func() {
		stored, receipt, snapshotErr := repo.GetTaskVisibilitySnapshot(ctx, workspaceID, task.ID)
		resultCh <- result{task: stored, receipt: receipt, err: snapshotErr}
	}()

	select {
	case <-readTask:
	case early := <-resultCh:
		t.Fatalf("snapshot returned before the task-read barrier: %+v", early)
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot did not reach the task-read barrier")
	}
	require.NoError(t, repo.DeleteTask(ctx, task.ID))
	releaseRead()

	var first result
	select {
	case first = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot did not complete after the task-read barrier was released")
	}
	require.NoError(t, first.err)
	require.NotNil(t, first.task)
	require.NotNil(t, first.receipt)
	require.Equal(t, "Before delete", first.task.Title)
	require.Equal(t, "active", first.receipt.Disposition)
	require.Equal(t, "create", first.receipt.Provenance)
	require.EqualValues(t, 1, first.receipt.Revision)

	repo.visibilitySnapshotAfterTaskRead = nil
	nextTask, nextReceipt, err := repo.GetTaskVisibilitySnapshot(ctx, workspaceID, task.ID)
	require.NoError(t, err)
	require.Nil(t, nextTask)
	require.NotNil(t, nextReceipt)
	require.Equal(t, "deleted", nextReceipt.Disposition)
	require.Equal(t, "delete", nextReceipt.Provenance)
	require.EqualValues(t, 2, nextReceipt.Revision)
}

func TestPostgresDeletedTaskLifecycleReceiptSurvivesRepositoryReopen(t *testing.T) {
	ctx := context.Background()
	openDB := newIsolatedPostgresMultiConnFactory(t, testutil.PostgresDSNFromEnv(t), 3)
	firstDB := openDB()
	first, err := NewWithDB(firstDB, firstDB, nil)
	require.NoError(t, err)
	workspaceID := "postgres-deleted-reopen-workspace"
	task := &models.Task{ID: "postgres-deleted-reopen-task", WorkspaceID: workspaceID, Title: "Deleted then reopened"}
	require.NoError(t, first.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Postgres deleted reopen"}))
	require.NoError(t, first.CreateTask(ctx, task))
	require.NoError(t, first.DeleteTask(ctx, task.ID))
	require.NoError(t, firstDB.Close())

	secondDB := openDB()
	second, err := NewWithDB(secondDB, secondDB, nil)
	require.NoError(t, err)
	stored, receipt, err := second.GetTaskVisibilitySnapshot(ctx, workspaceID, task.ID)
	require.NoError(t, err)
	require.Nil(t, stored)
	require.NotNil(t, receipt)
	require.Equal(t, "deleted", receipt.Disposition)
	require.Equal(t, "delete", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)
}

func TestPostgresTaskVisibilitySnapshotReadDoesNotMutateAllowedOrForeignScopedRows(t *testing.T) {
	ctx := context.Background()
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	workspaceID := "postgres-visibility-read-workspace"
	task := &models.Task{ID: "postgres-visibility-read-task", WorkspaceID: workspaceID, Title: "Read invariant"}
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Postgres visibility read"}))
	require.NoError(t, repo.CreateTask(ctx, task))
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "postgres-visibility-read-session", TaskID: task.ID, State: models.TaskSessionStateCompleted,
		StartedAt: now, CompletedAt: &now, UpdatedAt: now,
	}))
	_, err = db.Exec(db.Rebind(`
		INSERT INTO task_documents (id, task_id, key, type, title, content, author_kind, author_name, filename, mime_type, size_bytes, disk_path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "postgres-visibility-read-document", task.ID, "visibility", "plan", "Visibility", "content", "agent", "test", "", "", 0, "", now, now)
	require.NoError(t, err)
	_, err = db.Exec(db.Rebind(`
		INSERT INTO task_document_revisions (id, task_id, document_key, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
	`), "postgres-visibility-read-document-revision", task.ID, "visibility", 1, "Visibility", "content", "agent", "test", now, now)
	require.NoError(t, err)
	before := map[string][]string{
		"tasks":                   lifecycleTableRows(t, repo, "tasks", "id"),
		"task_lifecycle_receipts": lifecycleTableRows(t, repo, "task_lifecycle_receipts", "task_id"),
		"task_sessions":           lifecycleTableRows(t, repo, "task_sessions", "id"),
		"task_documents":          lifecycleTableRows(t, repo, "task_documents", "id"),
		"task_document_revisions": lifecycleTableRows(t, repo, "task_document_revisions", "id"),
	}
	stored, receipt, err := repo.GetTaskVisibilitySnapshot(ctx, workspaceID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.NotNil(t, receipt)
	foreignTask, foreignReceipt, err := repo.GetTaskVisibilitySnapshot(ctx, "other-workspace", task.ID)
	require.NoError(t, err)
	require.NotNil(t, foreignTask, "the repository returns the live row; service enforces foreign ownership")
	require.Nil(t, foreignReceipt, "receipt lookup is bound to the requested workspace")
	for table, beforeRows := range before {
		key := "id"
		if table == "task_lifecycle_receipts" {
			key = "task_id"
		}
		require.Equal(t, beforeRows, lifecycleTableRows(t, repo, table, key), table)
	}
}
