package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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

// TestService_BulkMoveTasksReordersAfterSourceStepDrift proves BulkMoveTasks's
// sibling of the fix above: a mid-batch source-step drift must correct the
// REQ-TASKS-KANBAN-TASK-REORDERING-001.29 submission order, not just which
// tasks move. orderTasksForBulkMove runs once before the lock (on the stale
// read) and again after (on acquireBulkMoveStepLocks' lock-corrected
// membership); only the second result may reach the dispatch loop.
//
// task-a starts in step-high (ordinal 10) and task-b starts in step-low
// (ordinal 0), so the stale pre-lock order is [task-b, task-a]. Before the
// lock is acquired, task-b drifts into step-highest (ordinal 20) — higher
// than task-a's step. The correct, lock-corrected order is therefore
// [task-a, task-b]. Dispatch order is observable via each task's resulting
// `position` in the (initially empty) target step: MoveTask assigns
// arrival positions in call order, so the first-dispatched task ends up
// with the lower position.
func TestService_BulkMoveTasksReordersAfterSourceStepDrift(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-low":     {ID: "step-low", WorkflowID: "wf-source", Name: "Low", Position: 0},
		"step-high":    {ID: "step-high", WorkflowID: "wf-source", Name: "High", Position: 10},
		"step-highest": {ID: "step-highest", WorkflowID: "wf-source", Name: "Highest", Position: 20},
		"step-target":  {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
	}})

	createMoveTask(t, ctx, repo, "task-a", "wf-source", "step-high", nil)
	createMoveTask(t, ctx, repo, "task-b", "wf-source", "step-low", nil)

	svc.bulkMoveBeforeLockForTest = func() {
		if _, err := svc.MoveTask(ctx, "task-b", "wf-source", "step-highest", 0); err != nil {
			t.Errorf("drift MoveTask: %v", err)
		}
	}
	t.Cleanup(func() { svc.bulkMoveBeforeLockForTest = nil })

	result, err := svc.BulkMoveTasks(ctx, "wf-source", "", "wf-target", "step-target")
	if err != nil {
		t.Fatalf("BulkMoveTasks: %v", err)
	}
	if result.MovedCount != 2 {
		t.Fatalf("MovedCount = %d, want 2", result.MovedCount)
	}

	taskA, err := repo.GetTask(ctx, "task-a")
	if err != nil {
		t.Fatalf("GetTask(task-a): %v", err)
	}
	taskB, err := repo.GetTask(ctx, "task-b")
	if err != nil {
		t.Fatalf("GetTask(task-b): %v", err)
	}
	if taskA.WorkflowStepID != "step-target" || taskB.WorkflowStepID != "step-target" {
		t.Fatalf("both tasks must land in step-target, got task-a=%q task-b=%q",
			taskA.WorkflowStepID, taskB.WorkflowStepID)
	}
	if taskA.Position >= taskB.Position {
		t.Fatalf("task-a.Position=%d, task-b.Position=%d: want task-a dispatched "+
			"(and so positioned) before task-b, since after the drift task-a's "+
			"source step (ordinal 10) ranks below task-b's drifted-to source "+
			"step (ordinal 20) — the stale pre-lock order would have dispatched "+
			"task-b first instead", taskA.Position, taskB.Position)
	}
}
