package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestService_BulkMoveTasksHoldsStepLockAcrossWholeBatch is BulkMoveTasks'
// sibling of TestService_BulkMoveSelectedTasksHoldsStepLockAcrossWholeBatch:
// the admin whole-workflow migration path must hold the same batch-spanning
// arrival lock, not just the per-call one MoveTask already takes.
func TestService_BulkMoveTasksHoldsStepLockAcrossWholeBatch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)

	createMoveTask(t, ctx, repo, "batch-1", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "batch-2", "wf-source", "step-source", nil)

	reachedMidBatch := make(chan struct{})
	proceedBatch := make(chan struct{})
	var hookMu sync.Mutex
	hookCalls := 0
	svc.bulkMoveAfterTaskForTest = func() {
		hookMu.Lock()
		hookCalls++
		first := hookCalls == 1
		hookMu.Unlock()
		if !first {
			return
		}
		close(reachedMidBatch)
		<-proceedBatch
	}
	t.Cleanup(func() { svc.bulkMoveAfterTaskForTest = nil })

	bulkDone := make(chan error, 1)
	go func() {
		_, err := svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-target", "step-target")
		bulkDone <- err
	}()

	select {
	case <-reachedMidBatch:
	case <-time.After(5 * time.Second):
		t.Fatal("bulk move never reached its mid-batch hook")
	}

	// A concurrent, unrelated arrival into the same target step while the
	// batch holds it mid-loop must block rather than slot in between
	// batch-1 and batch-2's positions.
	interloperDone := make(chan error, 1)
	go func() {
		interloperDone <- repo.CreateTask(ctx, &models.Task{
			ID: "interloper", WorkspaceID: "ws-1", WorkflowID: "wf-target",
			WorkflowStepID: "step-target", Title: "Interloper",
		})
	}()

	select {
	case <-interloperDone:
		t.Fatal("interloper arrival completed while the batch still held the target step's lock")
	case <-time.After(150 * time.Millisecond):
		// Expected: still blocked behind the batch's lock.
	}

	close(proceedBatch)

	if err := <-bulkDone; err != nil {
		t.Fatalf("BulkMoveTasks: %v", err)
	}
	if err := <-interloperDone; err != nil {
		t.Fatalf("interloper CreateTask: %v", err)
	}

	b1, err := repo.GetTask(ctx, "batch-1")
	if err != nil {
		t.Fatalf("GetTask(batch-1): %v", err)
	}
	b2, err := repo.GetTask(ctx, "batch-2")
	if err != nil {
		t.Fatalf("GetTask(batch-2): %v", err)
	}
	interloper, err := repo.GetTask(ctx, "interloper")
	if err != nil {
		t.Fatalf("GetTask(interloper): %v", err)
	}
	if b2.Position != b1.Position+1 {
		t.Fatalf("batch positions not consecutive: batch-1=%d, batch-2=%d (interloper=%d)",
			b1.Position, b2.Position, interloper.Position)
	}
}

// TestService_BulkMoveDoesNotDeadlockAgainstOppositeDirectionMove proves the
// system design's cross-step lock-ordering rule holds for a bulk move, not
// only a single MoveTask: two steps' arrival locks must always be acquired
// in the same ascending order, whichever caller acquires them.
//
// step-source sorts before step-target lexically. A bulk move from
// step-source to step-target that locked only its target up front, then
// picked up step-source per task inside its loop, would fix its acquisition
// order at target-then-source — the reverse of ascending whenever the
// source id sorts first, as it does here. An ordinary single move in the
// opposite direction (step-target -> step-source) correctly locks
// source-then-target. Interleaved, that is a classic AB-BA deadlock: the
// bulk move holds target and waits on source, the single move holds source
// and waits on target, and neither `sync.Mutex.Lock` call ever times out.
func TestService_BulkMoveDoesNotDeadlockAgainstOppositeDirectionMove(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)

	createMoveTask(t, ctx, repo, "batch-1", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "opposite-1", "wf-target", "step-target", nil)

	lockAcquired := make(chan struct{})
	proceed := make(chan struct{})
	svc.bulkMoveAfterLockForTest = func() {
		close(lockAcquired)
		<-proceed
	}
	t.Cleanup(func() { svc.bulkMoveAfterLockForTest = nil })

	bulkDone := make(chan error, 1)
	go func() {
		_, err := svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-target", "step-target")
		bulkDone <- err
	}()

	select {
	case <-lockAcquired:
	case <-time.After(5 * time.Second):
		t.Fatal("bulk move never reached its post-lock hook")
	}

	singleDone := make(chan error, 1)
	go func() {
		_, err := svc.MoveTask(ctx, "opposite-1", "wf-source", "step-source", 0)
		singleDone <- err
	}()

	// Give the opposite-direction move a chance to reach its own lock
	// acquisition (and, under the pre-fix code, its own half of the cycle)
	// before releasing the bulk move to attempt the rest of its locks.
	time.Sleep(50 * time.Millisecond)
	close(proceed)

	timeout := time.After(5 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case err := <-bulkDone:
			if err != nil {
				t.Fatalf("BulkMoveTasks: %v", err)
			}
		case err := <-singleDone:
			if err != nil {
				t.Fatalf("MoveTask: %v", err)
			}
		case <-timeout:
			t.Fatal("bulk move and the opposite-direction move deadlocked: neither completed within the timeout")
		}
	}

	moved, err := repo.GetTask(ctx, "batch-1")
	if err != nil {
		t.Fatalf("GetTask(batch-1): %v", err)
	}
	if moved.WorkflowStepID != "step-target" {
		t.Fatalf("batch-1 step = %q, want step-target", moved.WorkflowStepID)
	}
	swapped, err := repo.GetTask(ctx, "opposite-1")
	if err != nil {
		t.Fatalf("GetTask(opposite-1): %v", err)
	}
	if swapped.WorkflowStepID != "step-source" {
		t.Fatalf("opposite-1 step = %q, want step-source", swapped.WorkflowStepID)
	}
}
