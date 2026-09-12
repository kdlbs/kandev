package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestService_BulkMoveSelectedTasksRelocksAfterSourceStepDrift proves
// acquireBulkMoveStepLocks corrects for a source step that changes between
// the batch's pre-lock task read (validateSelectedMoveBatch,
// orderTasksForBulkMove) and its first LockStepArrivalsForBatch attempt.
//
// Locking only the stale, pre-drift step set would leave the batch holding
// a lock on a step the task has already left while MoveTask, later in the
// dispatch loop, correctly locks the task's real current step — a window
// where an unrelated mover already holding that real step and waiting on
// this batch's target step produces an AB-BA deadlock. Re-reading under the
// held lock and retrying on mismatch (bulkMoveBeforeLockForTest fires once,
// before the first attempt, letting the test perform the drift
// deterministically instead of racing a real goroutine for it) must instead
// converge on the task's true current step.
func TestService_BulkMoveSelectedTasksRelocksAfterSourceStepDrift(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)

	createMoveTask(t, ctx, repo, "batch-1", "wf-source", "step-source", nil)

	svc.bulkMoveBeforeLockForTest = func() {
		if _, err := svc.MoveTask(ctx, "batch-1", "wf-source", "step-review-target", 0); err != nil {
			t.Errorf("drift MoveTask: %v", err)
		}
	}
	t.Cleanup(func() { svc.bulkMoveBeforeLockForTest = nil })

	lockAcquired := make(chan struct{})
	proceed := make(chan struct{})
	svc.bulkMoveAfterLockForTest = func() {
		close(lockAcquired)
		<-proceed
	}
	t.Cleanup(func() { svc.bulkMoveAfterLockForTest = nil })

	bulkDone := make(chan error, 1)
	go func() {
		_, err := svc.BulkMoveSelectedTasks(ctx, []string{"batch-1"}, "wf-target", "step-target")
		bulkDone <- err
	}()

	select {
	case <-lockAcquired:
	case <-time.After(5 * time.Second):
		t.Fatal("bulk move never reached its post-lock hook")
	}

	// If acquireBulkMoveStepLocks correctly re-locked onto
	// step-review-target (batch-1's real current step after the drift)
	// rather than the stale step-source read before the drift, a
	// concurrent arrival into step-review-target must block behind it.
	interloperDone := make(chan error, 1)
	go func() {
		interloperDone <- repo.CreateTask(ctx, &models.Task{
			ID: "interloper", WorkspaceID: "ws-1", WorkflowID: "wf-source",
			WorkflowStepID: "step-review-target", Title: "Interloper",
		})
	}()

	select {
	case <-interloperDone:
		t.Fatal("interloper arrival into the drifted-to step completed while the batch should still hold its lock")
	case <-time.After(150 * time.Millisecond):
		// Expected: still blocked behind the batch's (corrected) lock.
	}

	close(proceed)

	if err := <-bulkDone; err != nil {
		t.Fatalf("BulkMoveSelectedTasks: %v", err)
	}
	if err := <-interloperDone; err != nil {
		t.Fatalf("interloper CreateTask: %v", err)
	}

	moved, err := repo.GetTask(ctx, "batch-1")
	if err != nil {
		t.Fatalf("GetTask(batch-1): %v", err)
	}
	if moved.WorkflowStepID != "step-target" {
		t.Fatalf("batch-1 step = %q, want step-target", moved.WorkflowStepID)
	}
}

// TestService_BulkMoveSelectedTasksMovesTaskThatDriftedOutOfTargetBeforeLock
// proves the dispatch loop's "already at the target step, skip it" check
// (BulkMoveSelectedTasks's intentional behavior for a batch that includes
// tasks already where the caller wants them) uses the batch's real,
// lock-corrected membership rather than the stale pre-lock read.
//
// A task already at the target step when validateSelectedMoveBatch reads the
// batch is a legitimate member of that batch (BulkMoveSelectedTasks's own
// doc comment: "tasks already in the target step are skipped"). If a
// concurrent move then carries that same task OUT of the target step before
// this batch's locks are acquired, acquireBulkMoveStepLocks correctly
// re-locks its new real step — but the task no longer belongs in the
// "already there" case: the caller still wants it at the target, and it no
// longer is. The dispatch loop must dispatch it, not skip it.
func TestService_BulkMoveSelectedTasksMovesTaskThatDriftedOutOfTargetBeforeLock(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)

	createMoveTask(t, ctx, repo, "batch-1", "wf-target", "step-target", nil)

	svc.bulkMoveBeforeLockForTest = func() {
		if _, err := svc.MoveTask(ctx, "batch-1", "wf-source", "step-source", 0); err != nil {
			t.Errorf("drift MoveTask: %v", err)
		}
	}
	t.Cleanup(func() { svc.bulkMoveBeforeLockForTest = nil })

	if _, err := svc.BulkMoveSelectedTasks(ctx, []string{"batch-1"}, "wf-target", "step-target"); err != nil {
		t.Fatalf("BulkMoveSelectedTasks: %v", err)
	}

	moved, err := repo.GetTask(ctx, "batch-1")
	if err != nil {
		t.Fatalf("GetTask(batch-1): %v", err)
	}
	if moved.WorkflowStepID != "step-target" {
		t.Fatalf("batch-1 step = %q, want step-target (task drifted out of the "+
			"target before the lock was acquired; it must still be moved back "+
			"in, not silently skipped as if it had never left)", moved.WorkflowStepID)
	}
}
