package sqlite

// TestPostgresLockTaskStepForWriteReleasesStaleStepLockOnRetry and
// TestPostgresLockTaskStepForWriteReleasesStaleStepLockWhenTaskLeavesStep
// prove lockTaskStepForWrite's confirm-and-retry loop releases a stale step's
// FOR UPDATE lock (via ROLLBACK TO SAVEPOINT) on both outcomes of a mismatch —
// the task moved to a different step, or it left its step entirely — before
// returning or retrying, so it never holds more than one workflow_steps row
// lock at a time. Every other site in this package that locks two steps in
// one transaction sorts them ascending first (see lockWorkflowStepsForAdmission),
// specifically to avoid an AB-BA deadlock against another such locker. Before
// this fix, either outcome kept the stale step's lock held — Postgres has no
// per-row unlock short of a savepoint rollback — so the transaction could end
// up holding a step lock nothing still needed, in discovery order rather than
// sorted order: a real deadlock risk against a concurrent
// updateTaskWithWorkflowStepAdmission cross-step move over the same two steps.
//
// TestPostgresLockTaskStepForWriteRetryLeavesNoLockForConcurrentCrossStepMove
// is a weaker, complementary check, not a literal deadlock reproduction: since
// the fix above means the retry never holds more than one step lock, it can
// never actually contend against a concurrent sorted-order locker in this
// harness (the retry goroutine is deliberately paused holding zero locks at
// the moment the concurrent move runs), so it cannot raise Postgres's own
// SQLSTATE 40P01. What it shows instead is that the retry leaves no lock
// behind for an unrelated, correctly-ordered mover to trip over: a genuine
// cross-step move over the very same two steps completes promptly while the
// retry sits paused. Run against the pre-fix code, the equivalent scenario
// doesn't raise 40P01 either — it hangs, because the retry goroutine is
// parked on this harness's own pause channel before it ever issues the second
// lock request Postgres's cycle detector would need to see — so a pre-fix run
// times out rather than surfacing a captured deadlock error.
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

// TestPostgresLockTaskStepForWriteReleasesStaleStepLockWhenTaskLeavesStep proves
// the confirming re-read's third outcome — the task no longer belongs to any
// step at all (workflow_step_id cleared, e.g. by a concurrent
// RemoveTaskFromWorkflow) rather than having moved to a different one — also
// releases the stale step lock instead of leaving it held for the rest of the
// caller's transaction. This is the same savepoint-lifecycle bug class the
// mismatch-retry path was already fixed for: readTaskWorkflowStepID maps both
// "no such task" and "workflow_step_id is empty" to (found=false, err=nil), so
// treating that only as "return, nothing more to do" without also rolling back
// the just-opened savepoint left the lock dangling.
func TestPostgresLockTaskStepForWriteReleasesStaleStepLockWhenTaskLeavesStep(t *testing.T) {
	const stepA = "leaves-step-a"
	repo := seedVisibilityLockFixture(t, "leaves-step-ws", "leaves-step-workflow", stepA)
	ctx := context.Background()

	task := &models.Task{
		ID: "leaves-step-task", WorkspaceID: "leaves-step-ws", WorkflowID: "leaves-step-workflow",
		WorkflowStepID: stepA, Title: "Leaving", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	paused := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFn := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFn)

	calls := 0
	repo.taskStepLockBeforeAcquireHook = func(candidateStepID string) {
		calls++
		if calls != 1 {
			t.Errorf("expected exactly one lock attempt (no step to retry against once the "+
				"task has left it), got attempt %d for %q", calls, candidateStepID)
			return
		}
		if candidateStepID != stepA {
			t.Errorf("candidate step = %q, want %q", candidateStepID, stepA)
		}
		close(paused)
		<-release
	}
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

	<-paused // read stepA, about to lock it

	// Simulate a concurrent RemoveTaskFromWorkflow: detach the task from its
	// step entirely. Runs on a separate pooled connection and autocommits
	// immediately, so it's fully visible once release() lets the paused
	// goroutine proceed to its confirming FOR UPDATE read.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workflow_step_id = '' WHERE id = ?`,
	), task.ID); err != nil {
		t.Fatalf("simulate concurrent detach: %v", err)
	}
	releaseFn()

	if err := <-lockDone; err != nil {
		t.Fatalf("lockTaskStepForWrite: %v", err)
	}

	// If stepA's lock was released (rolled back to the savepoint) once the
	// confirming read found no step left to protect, a second connection can
	// lock it immediately with FOR UPDATE NOWAIT. If it's still held, this
	// errors instead of blocking indefinitely, so the assertion is immediate
	// either way.
	var lockedID string
	probeErr := repo.db.QueryRowContext(ctx, repo.db.Rebind(
		`SELECT id FROM workflow_steps WHERE id = ? FOR UPDATE NOWAIT`,
	), stepA).Scan(&lockedID)
	if probeErr != nil {
		t.Fatalf("stepA should be free once the task left it, but a second connection could not "+
			"acquire it NOWAIT (the stale lock is still held): %v", probeErr)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestPostgresLockTaskStepForWriteRetryLeavesNoLockForConcurrentCrossStepMove(t *testing.T) {
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
	// has already released stepB by this point, so Op1 holds no lock here.

	// Op2: a genuine cross-step move of a DIFFERENT task, which locks stepA
	// then stepB (updateTaskWithWorkflowStepAdmission's own established
	// ascending order). Op1 holds nothing at this point (see above), so this
	// is not contended and completing promptly is expected, not evidence of
	// deadlock avoidance under real contention — see the file header for what
	// this test does and doesn't prove.
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
		t.Fatalf("cross-step move of Y did not complete within 5s — Op1 holds no lock at this point, " +
			"so this indicates a hang unrelated to Postgres's own deadlock detection, not a captured 40P01")
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
