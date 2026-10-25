package sqlite

// TestPostgresWorkspaceCascadeDeleteLocksStepAgainstConcurrentReorder proves
// DeleteWorkspaceCascade locks a workspace's workflow_steps rows (via
// lockWorkflowStepForWrite) BEFORE it locks or deletes any task row, so a
// concurrent ReorderStepTasks of one of those steps cannot interleave with
// the cascade in a way that inverts this package's established lock order.
//
// Every other writer that touches both a step and its tasks in the same
// transaction (ReorderStepTasks, lockTaskStepForWrite's callers: ArchiveTask,
// ArchiveTaskIfActive, DeleteTask, UnarchiveTask, UnarchiveTaskByCascade)
// locks the step first and task rows second. Before this fix,
// deleteWorkspaceCascade inverted that order: it locked task rows (sorted by
// id, to establish task-row -> queue-session order) and deleted them BEFORE
// ever touching workflow_steps, which it only locked implicitly and late, via
// its closing `DELETE FROM workflow_steps ...`. Two transactions acquiring
// the same two resources in opposite orders is the textbook precondition for
// a real Postgres deadlock: a reorder holding the step lock while waiting on
// a task row the cascade already holds, and the cascade holding that task row
// while waiting on the step lock the reorder already holds, with neither able
// to proceed and Postgres left to abort one after its deadlock_timeout.
//
// This test does not need to provoke the deadlock detector itself (timing-
// dependent and slow); it proves the ordering fix directly, the same way the
// sibling archive/delete/unarchive tests in task_visibility_lock_postgres_test.go
// do: pause a reorder holding stepID's row lock via reorderPreWriteHook, run
// DeleteWorkspaceCascade concurrently, and observe — deterministically, since
// a transaction blocked on a Postgres row lock cannot have committed
// regardless of timing — that the workspace row is still present while the
// reorder is still paused. That is only possible if the cascade is blocked
// behind the step lock, i.e. it now acquires the step lock before proceeding
// to the task rows and the delete, matching ReorderStepTasks' own order.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestPostgresWorkspaceCascadeDeleteLocksStepAgainstConcurrentReorder(t *testing.T) {
	const step = "cascade-lock-step"
	const wsID = "cascade-lock-ws"
	const workflowID = "cascade-lock-workflow"
	repo := seedVisibilityLockFixture(t, wsID, workflowID, step)
	ctx := context.Background()

	staying1 := &models.Task{
		ID: "cascade-lock-stays-1", WorkspaceID: wsID, WorkflowID: workflowID,
		WorkflowStepID: step, Title: "Stays 1", WIPAdmitted: true,
	}
	staying2 := &models.Task{
		ID: "cascade-lock-stays-2", WorkspaceID: wsID, WorkflowID: workflowID,
		WorkflowStepID: step, Title: "Stays 2", WIPAdmitted: true,
	}
	for _, task := range []*models.Task{staying1, staying2} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}

	paused, release := armReorderPause(t, repo)
	t.Cleanup(func() { repo.reorderPreWriteHook = nil })

	var wg sync.WaitGroup
	var reorderErr, cascadeErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, reorderErr = repo.ReorderStepTasks(ctx, step, ReorderBandAdmitted, []string{staying1.ID, staying2.ID})
	}()

	<-paused // reorder holds step's row lock, has already read the live membership

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, cascadeErr = repo.DeleteWorkspaceCascade(ctx, wsID)
	}()

	// Give the cascade goroutine time to either run to completion (unfixed
	// code, no step lock ahead of the task rows/delete) or register its wait
	// against reorder's held step lock (fixed code) before we peek at
	// committed state.
	time.Sleep(200 * time.Millisecond)

	var workspaceStillExists bool
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT EXISTS (SELECT 1 FROM workspaces WHERE id = ?)`,
	), wsID).Scan(&workspaceStillExists); err != nil {
		t.Fatalf("peek workspace existence mid-pause: %v", err)
	}
	if !workspaceStillExists {
		t.Fatalf("DeleteWorkspaceCascade committed while the reorder still held step %q's lock: the cascade "+
			"must lock the workspace's steps before its task rows and delete, so it waits behind the reorder "+
			"instead of acquiring the same two resources in the opposite order", step)
	}

	release()
	wg.Wait()

	if cascadeErr != nil {
		t.Fatalf("workspace cascade delete must always succeed: %v", cascadeErr)
	}
	if reorderErr != nil && !errors.Is(reorderErr, repoerrors.ErrStepChanged) {
		t.Fatalf("reorder error = %v, want nil or ErrStepChanged", reorderErr)
	}

	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT EXISTS (SELECT 1 FROM workspaces WHERE id = ?)`,
	), wsID).Scan(&workspaceStillExists); err != nil {
		t.Fatalf("check workspace existence after release: %v", err)
	}
	if workspaceStillExists {
		t.Fatalf("workspace %q was not deleted after the cascade completed", wsID)
	}
}
