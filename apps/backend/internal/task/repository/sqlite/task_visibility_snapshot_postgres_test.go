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
