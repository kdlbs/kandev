package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestService_BulkMoveSelectedTasksHoldsStepLockAcrossWholeBatch proves
// REQ-TASKS-KANBAN-TASK-REORDERING-001.29's batch-scoped consecutiveness:
// BulkMoveSelectedTasks must hold the target step's arrival-position lock
// across its whole sequential dispatch loop, not just within each
// individual MoveTask call's own transaction. Without that, an unrelated
// concurrent arrival (another create, move, WIP promotion, or automatic
// transition) into the same target step can land in the middle of the
// batch's own sequence, breaking "Their position values shall be
// consecutive."
func TestService_BulkMoveSelectedTasksHoldsStepLockAcrossWholeBatch(t *testing.T) {
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

	bulkDone := make(chan error, 1)
	go func() {
		_, err := svc.BulkMoveSelectedTasks(ctx, []string{"batch-1", "batch-2"}, "wf-target", "step-target")
		bulkDone <- err
	}()

	select {
	case <-reachedMidBatch:
	case <-time.After(5 * time.Second):
		t.Fatal("bulk move never reached its mid-batch hook")
	}

	// Attempt a concurrent, unrelated arrival into the same target step
	// while the batch holds it mid-loop. It must block rather than slot in
	// between batch-1 and batch-2's positions.
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
		t.Fatalf("BulkMoveSelectedTasks: %v", err)
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
