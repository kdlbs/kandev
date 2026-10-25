package sqlite

// PostgreSQL coverage for the workspace-source parent guard. The guard uses
// a no-op UPDATE and RowsAffected to hold the target task row lock until the
// source transaction commits, so SQLite-only coverage cannot prove the
// cross-connection serialization behavior.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresWorkspaceSourceParentGuard(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-parent-guard-pg")

	t.Run("matching and stale predicates", func(t *testing.T) {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: "task-parent-guard-pg", WorkspaceID: "workspace-parent-guard-pg", ParentID: "parent-a", Title: "Child",
		}); err != nil {
			t.Fatalf("seed task: %v", err)
		}

		if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
			TaskID: "task-parent-guard-pg", ExpectedParentID: "parent-a", ExpectedParentWorkspaceID: "workspace-parent-guard-pg",
			Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{LocalPath: "/canonical/approved", DisplayName: "approved"}}},
		}); err != nil {
			t.Fatalf("matching parent write: %v", err)
		}

		if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
			TaskID: "task-parent-guard-pg", ExpectedParentID: "stale-parent", ExpectedParentWorkspaceID: "workspace-parent-guard-pg",
			Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{LocalPath: "/canonical/rejected", DisplayName: "rejected"}}},
		}); !errors.Is(err, repoerrors.ErrTaskParentMismatch) {
			t.Fatalf("stale parent error = %v, want parent mismatch", err)
		}
		folders, err := repo.ListTaskWorkspaceFolders(ctx, "task-parent-guard-pg")
		if err != nil {
			t.Fatalf("list folders: %v", err)
		}
		if len(folders) != 1 || folders[0].DisplayName != "approved" {
			t.Fatalf("folders after stale write = %#v, want only approved", folders)
		}
	})

	t.Run("source commit serializes reparent", func(t *testing.T) {
		const taskID = "task-parent-guard-source-wins-pg"
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-parent-guard-pg", ParentID: "parent-a", Title: "Source wins"}); err != nil {
			t.Fatalf("seed task: %v", err)
		}

		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("begin source transaction: %v", err)
		}
		batch := &models.WorkspaceSourceBatch{
			TaskID: taskID, ExpectedParentID: "parent-a", ExpectedParentWorkspaceID: "workspace-parent-guard-pg",
			Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{LocalPath: "/canonical/source-wins", DisplayName: "source-wins"}}},
		}
		if err := repo.createWorkspaceSourceBatchTx(ctx, tx, batch); err != nil {
			_ = tx.Rollback()
			t.Fatalf("prepare source transaction: %v", err)
		}

		reparentStarted := make(chan struct{})
		reparentDone := make(chan error, 1)
		go func() {
			close(reparentStarted)
			_, updateErr := db.ExecContext(ctx, db.Rebind(`UPDATE tasks SET parent_id = ? WHERE id = ?`), "parent-b", taskID)
			reparentDone <- updateErr
		}()
		<-reparentStarted
		assertBlockedUntilCommit(t, reparentDone, "reparent")
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit source transaction: %v", err)
		}
		if err := waitForConcurrentResult(reparentDone, "reparent"); err != nil {
			t.Fatalf("reparent after source commit: %v", err)
		}

		folders, err := repo.ListTaskWorkspaceFolders(ctx, taskID)
		if err != nil {
			t.Fatalf("list source-wins folders: %v", err)
		}
		if len(folders) != 1 || folders[0].DisplayName != "source-wins" {
			t.Fatalf("source-wins folders = %#v, want one committed folder", folders)
		}
	})

	t.Run("reparent commit rejects stale source", func(t *testing.T) {
		const taskID = "task-parent-guard-reparent-wins-pg"
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-parent-guard-pg", ParentID: "parent-a", Title: "Reparent wins"}); err != nil {
			t.Fatalf("seed task: %v", err)
		}

		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("begin reparent transaction: %v", err)
		}
		if _, err := tx.ExecContext(ctx, db.Rebind(`UPDATE tasks SET parent_id = ? WHERE id = ?`), "parent-b", taskID); err != nil {
			_ = tx.Rollback()
			t.Fatalf("prepare reparent transaction: %v", err)
		}

		sourceStarted := make(chan struct{})
		sourceDone := make(chan error, 1)
		go func() {
			close(sourceStarted)
			sourceDone <- repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
				TaskID: taskID, ExpectedParentID: "parent-a", ExpectedParentWorkspaceID: "workspace-parent-guard-pg",
				Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{LocalPath: "/canonical/reparent-wins", DisplayName: "reparent-wins"}}},
			})
		}()
		<-sourceStarted
		assertBlockedUntilCommit(t, sourceDone, "source")
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit reparent transaction: %v", err)
		}
		if err := waitForConcurrentResult(sourceDone, "source"); !errors.Is(err, repoerrors.ErrTaskParentMismatch) {
			t.Fatalf("source after reparent commit error = %v, want parent mismatch", err)
		}

		folders, err := repo.ListTaskWorkspaceFolders(ctx, taskID)
		if err != nil {
			t.Fatalf("list reparent-wins folders: %v", err)
		}
		if len(folders) != 0 {
			t.Fatalf("reparent-wins folders = %#v, want no write", folders)
		}
	})
}

func assertBlockedUntilCommit(t *testing.T, result <-chan error, operation string) {
	t.Helper()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-result:
		t.Fatalf("%s completed before the locking transaction committed: %v", operation, err)
	case <-timer.C:
	}
}

func waitForConcurrentResult(result <-chan error, operation string) error {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return errors.New(operation + " did not complete after the locking transaction committed")
	}
}
