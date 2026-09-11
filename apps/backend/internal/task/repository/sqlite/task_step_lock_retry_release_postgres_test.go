package sqlite

// TestPostgresLockTaskStepForWriteReleasesStaleStepLockOnRetry and
// TestPostgresLockTaskStepForWriteRetryDoesNotDeadlockConcurrentCrossStepMove
// prove lockTaskStepForWrite's confirm-and-retry loop never holds more than
// one workflow_steps row lock at a time. Every other site in this package
// that locks two steps in one transaction sorts them ascending first (see
// lockWorkflowStepsForAdmission), specifically to avoid an AB-BA deadlock
// against another such locker. Before this fix, a retry (triggered when the
// task moves between the loop's unlocked read and its lock acquisition) kept
// the stale step's FOR UPDATE lock held — Postgres has no per-row unlock
// short of a savepoint rollback — while going on to lock the task's new
// step, so the transaction could end up holding two step locks in discovery
// order instead of sorted order. A concurrent updateTaskWithWorkflowStepAdmission
// cross-step move over the same two steps (always sorted ascending) can then
// deadlock against it: reproduced by hand and confirmed independently.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// armTwoStepLockHook arms repo.taskStepLockBeforeAcquireHook to pause on its
// first and second invocation, asserting the candidate step id each time.
// Returns the two pause channels (closed once that invocation is paused)
// and idempotent release functions, both also registered as t.Cleanup so a
// t.Fatalf between pausing and releasing cannot leak the paused goroutine.
func armTwoStepLockHook(t *testing.T, repo *Repository, wantFirst, wantSecond string) (pause1, pause2 chan struct{}, release1, release2 func()) {
	t.Helper()
	p1 := make(chan struct{})
	p2 := make(chan struct{})
	r1 := make(chan struct{})
	r2 := make(chan struct{})
	var release1Once, release2Once sync.Once
	release1 = func() { release1Once.Do(func() { close(r1) }) }
	release2 = func() { release2Once.Do(func() { close(r2) }) }
	t.Cleanup(release1)
	t.Cleanup(release2)

	var mu sync.Mutex
	calls := 0
	repo.taskStepLockBeforeAcquireHook = func(candidateStepID string) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		switch n {
		case 1:
			if candidateStepID != wantFirst {
				t.Errorf("first candidate step = %q, want %q", candidateStepID, wantFirst)
			}
			close(p1)
			<-r1
		case 2:
			if candidateStepID != wantSecond {
				t.Errorf("second candidate step = %q, want %q", candidateStepID, wantSecond)
			}
			close(p2)
			<-r2
		}
	}
	return p1, p2, release1, release2
}

func TestPostgresLockTaskStepForWriteReleasesStaleStepLockOnRetry(t *testing.T) {
	const stepA = "retry-release-step-a"
	const stepB = "retry-release-step-b"
	repo := seedVisibilityLockFixture(t, "retry-release-ws", "retry-release-workflow", stepA)
	ctx := context.Background()

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?)
	`), stepB, "retry-release-workflow", stepB, 1); err != nil {
		t.Fatalf("seed step B: %v", err)
	}

	task := &models.Task{
		ID: "retry-release-task", WorkspaceID: "retry-release-ws", WorkflowID: "retry-release-workflow",
		WorkflowStepID: stepB, Title: "Moving", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	pause1, pause2, release1, release2 := armTwoStepLockHook(t, repo, stepB, stepA)
	t.Cleanup(func() { repo.taskStepLockBeforeAcquireHook = nil })

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	lockDone := make(chan error, 1)
	go func() {
		lockDone <- repo.lockTaskStepForWrite(ctx, tx, task.ID)
	}()

	<-pause1 // read stepB (the task's original step), about to lock it

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`,
	), stepA, task.ID); err != nil {
		t.Fatalf("simulate concurrent move: %v", err)
	}
	release1()

	<-pause2 // retry: read stepA, about to lock it

	// If the retry released stepB's now-stale lock (via a savepoint rollback)
	// before moving on, a second connection can lock it immediately with
	// FOR UPDATE NOWAIT. If the stale lock is still held, this errors instead
	// of blocking indefinitely, so the assertion is immediate either way.
	var lockedID string
	probeErr := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT id FROM workflow_steps WHERE id = ? FOR UPDATE NOWAIT`,
	), stepB).Scan(&lockedID)
	if probeErr != nil {
		t.Fatalf("stepB should be free once the retry moved on to stepA, but a second connection could not "+
			"acquire it NOWAIT (the stale lock from the first attempt is still held): %v", probeErr)
	}

	release2()
	if err := <-lockDone; err != nil {
		t.Fatalf("lockTaskStepForWrite: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgresLockTaskStepForWriteRetryDoesNotDeadlockConcurrentCrossStepMove(t *testing.T) {
	const stepA = "retry-deadlock-step-a"
	const stepB = "retry-deadlock-step-b"
	repo := seedVisibilityLockFixture(t, "retry-deadlock-ws", "retry-deadlock-workflow", stepA)
	ctx := context.Background()

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?)
	`), stepB, "retry-deadlock-workflow", stepB, 1); err != nil {
		t.Fatalf("seed step B: %v", err)
	}

	x := &models.Task{
		ID: "retry-deadlock-task-x", WorkspaceID: "retry-deadlock-ws", WorkflowID: "retry-deadlock-workflow",
		WorkflowStepID: stepB, Title: "X", WIPAdmitted: true,
	}
	y := &models.Task{
		ID: "retry-deadlock-task-y", WorkspaceID: "retry-deadlock-ws", WorkflowID: "retry-deadlock-workflow",
		WorkflowStepID: stepA, Title: "Y", WIPAdmitted: true,
	}
	for _, task := range []*models.Task{x, y} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}

	pause1, pause2, release1, release2 := armTwoStepLockHook(t, repo, stepB, stepA)
	t.Cleanup(func() { repo.taskStepLockBeforeAcquireHook = nil })

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	lockDone := make(chan error, 1)
	go func() {
		lockDone <- repo.lockTaskStepForWrite(ctx, tx, x.ID)
	}()

	<-pause1 // Op1 read stepB (X's original step), about to lock it

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`,
	), stepA, x.ID); err != nil {
		t.Fatalf("simulate concurrent move of X: %v", err)
	}
	release1()

	<-pause2 // Op1's retry read stepA, about to lock it; a fixed implementation
	// has already released stepB by this point.

	// Op2: a genuine cross-step move of a DIFFERENT task, which locks stepA
	// then stepB (updateTaskWithWorkflowStepAdmission's own established
	// ascending order). Pre-fix, Op1 would still be holding stepB here, so
	// Op2 blocks on stepB while Op1's very next step is to block on stepA
	// (which Op2 already holds) — the AB-BA deadlock this test targets.
	moveDone := make(chan error, 1)
	go func() {
		_, moveErr := repo.UpdateTaskWithWorkflowStepAdmission(ctx, y, stepA, stepB, 0)
		moveDone <- moveErr
	}()

	select {
	case err := <-moveDone:
		if err != nil {
			t.Fatalf("cross-step move of Y: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("cross-step move of Y did not complete within 5s — consistent with a deadlock against " +
			"lockTaskStepForWrite's retry")
	}

	release2()

	select {
	case err := <-lockDone:
		if err != nil {
			t.Fatalf("lockTaskStepForWrite: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("lockTaskStepForWrite did not complete within 5s")
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	movedY, err := repo.GetTask(ctx, y.ID)
	if err != nil {
		t.Fatalf("reload Y: %v", err)
	}
	if movedY.WorkflowStepID != stepB {
		t.Fatalf("Y ended up in step %q, want %q", movedY.WorkflowStepID, stepB)
	}
}
