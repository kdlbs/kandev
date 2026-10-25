package sqlite

// TestPostgresLockTaskStepForWriteHoldsTaskRowWhenStepGoneForGood,
// TestPostgresLockTaskStepForWriteHoldsTaskRowWhenNeverInAStep, and
// TestPostgresLockTaskStepForWriteLocksNewStepWhenReattachedDuringGap close
// a gap left by the fix in task_step_lock_retry_release_postgres_test.go
// (Review round 9): ROLLBACK TO SAVEPOINT releases every lock taken since
// the savepoint, not only the step lock it was meant to drop. Releasing a
// stale step's lock on the confirming read's own found==false outcome also
// freed that read's FOR UPDATE lock on the task's own row, so
// lockTaskStepForWrite returned success with nothing locked at all. A
// concurrent transaction could then reattach the task to a brand-new step
// before the caller's own mutation (archive, delete, unarchive) ran, and
// that mutation would proceed without ever locking the new step -
// reopening the exact race this whole function exists to prevent:
// REQ-TASKS-KANBAN-TASK-REORDERING-001.15 ("a hidden task's position shall
// not be rewritten") and .26 ("any membership change in the window
// conflicts, whatever caused it"). The same gap existed, unreachable by any
// prior review, on the much older early-return path for a task that has no
// step on its very first (unlocked) read - a task that never had a step
// takes the same finding's remedy.
//
// lockTaskRowIfStepless closes both: whenever lockTaskStepForWrite
// concludes a task currently has no step, it re-verifies under a FOR
// UPDATE lock on the task's OWN row - never a step, so this cannot invert
// the step-lock-before-task-row-lock order lockTaskStepForWrite's own doc
// comment requires - and either keeps that lock through return (still no
// step: the caller's mutation is now protected against any later
// reattachment) or releases it and retries the normal step-lock sequence
// against the task's new step (reattached: there IS a step to protect
// after all).
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// TestPostgresLockTaskStepForWriteHoldsTaskRowWhenStepGoneForGood proves the
// literal scenario this round's finding was reproduced with: a task leaves
// its step while lockTaskStepForWrite's confirming read is waiting on that
// step's lock, and is never reattached. Releasing the stale step's lock
// must not also leave the task's own row free for a concurrent writer.
func TestPostgresLockTaskStepForWriteHoldsTaskRowWhenStepGoneForGood(t *testing.T) {
	const step = "reattach-gone-step-a"
	repo := seedVisibilityLockFixture(t, "reattach-gone-ws", "reattach-gone-workflow", step)
	ctx := context.Background()

	task := &models.Task{
		ID: "reattach-gone-task", WorkspaceID: "reattach-gone-ws", WorkflowID: "reattach-gone-workflow",
		WorkflowStepID: step, Title: "Leaving for good", WIPAdmitted: true,
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
			t.Errorf("expected exactly one step-lock attempt (the task never gets a step back), got attempt %d for %q", calls, candidateStepID)
			return
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
	<-paused

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workflow_step_id = '' WHERE id = ?`,
	), task.ID); err != nil {
		t.Fatalf("simulate concurrent detach: %v", err)
	}
	releaseFn()

	if err := <-lockDone; err != nil {
		t.Fatalf("lockTaskStepForWrite: %v", err)
	}

	assertTaskRowBlocksConcurrentWrite(t, ctx, repo, tx, task.ID)
}

// TestPostgresLockTaskStepForWriteHoldsTaskRowWhenNeverInAStep proves the
// same invariant holds on the much older path: a task that has no step on
// lockTaskStepForWrite's very first (unlocked) read - same shape as a
// config task, or one already detached before the caller was ever invoked
// - must still come out of the function with its own row locked.
func TestPostgresLockTaskStepForWriteHoldsTaskRowWhenNeverInAStep(t *testing.T) {
	const step = "reattach-never-step-a"
	repo := seedVisibilityLockFixture(t, "reattach-never-ws", "reattach-never-workflow", step)
	ctx := context.Background()

	task := &models.Task{
		ID: "reattach-never-task", WorkspaceID: "reattach-never-ws", WorkflowID: "reattach-never-workflow",
		WorkflowStepID: "", Title: "Never in a step", WIPAdmitted: false,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed stepless task: %v", err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := repo.lockTaskStepForWrite(ctx, tx, task.ID); err != nil {
		t.Fatalf("lockTaskStepForWrite: %v", err)
	}

	assertTaskRowBlocksConcurrentWrite(t, ctx, repo, tx, task.ID)
}

// TestPostgresLockTaskStepForWriteLocksNewStepWhenReattachedDuringGap proves
// the other half: if the task is reattached to a different step in the gap
// lockTaskRowIfStepless exists to close, lockTaskStepForWrite must lock
// that new step rather than conclude there is nothing to protect.
func TestPostgresLockTaskStepForWriteLocksNewStepWhenReattachedDuringGap(t *testing.T) {
	const stepA = "reattach-gap-step-a"
	const stepB = "reattach-gap-step-b"
	repo := seedVisibilityLockFixture(t, "reattach-gap-ws", "reattach-gap-workflow", stepA)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position)
		VALUES (?, ?, ?, ?)
	`), stepB, "reattach-gap-workflow", stepB, 1); err != nil {
		t.Fatalf("seed step B: %v", err)
	}

	task := &models.Task{
		ID: "reattach-gap-task", WorkspaceID: "reattach-gap-ws", WorkflowID: "reattach-gap-workflow",
		WorkflowStepID: stepA, Title: "Reattaching", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	// Detach before the call even starts, so the unlocked first read already
	// finds no step and the race lands in lockTaskRowIfStepless's own
	// confirming lock rather than needing to also race the initial step
	// lock.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workflow_step_id = '' WHERE id = ?`,
	), task.ID); err != nil {
		t.Fatalf("detach before the call: %v", err)
	}

	paused := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFn := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFn)
	calls := 0
	repo.taskRowReconfirmHook = func() {
		calls++
		if calls != 1 {
			return
		}
		close(paused)
		<-release
	}
	t.Cleanup(func() { repo.taskRowReconfirmHook = nil })

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	lockDone := make(chan error, 1)
	go func() {
		lockDone <- repo.lockTaskStepForWrite(ctx, tx, task.ID)
	}()
	<-paused

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`,
	), stepB, task.ID); err != nil {
		t.Fatalf("simulate concurrent reattach: %v", err)
	}
	releaseFn()

	if err := <-lockDone; err != nil {
		t.Fatalf("lockTaskStepForWrite: %v", err)
	}

	// stepB must now be the one locked: a concurrent ReorderStepTasks(stepB)
	// should block until our tx commits.
	reorderDone := make(chan error, 1)
	go func() {
		_, _, err := repo.ReorderStepTasks(ctx, stepB, ReorderBandAdmitted, []string{task.ID})
		reorderDone <- err
	}()
	select {
	case err := <-reorderDone:
		t.Fatalf("ReorderStepTasks(stepB) should have blocked behind the caller's lock on stepB, got: %v", err)
	case <-time.After(300 * time.Millisecond):
		// expected: still blocked while our tx is open
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-reorderDone; err != nil {
		t.Fatalf("ReorderStepTasks(stepB) after commit: %v", err)
	}
}

// assertTaskRowBlocksConcurrentWrite proves taskID's own row is locked by
// tx: a second connection's write to it must block until tx commits. A
// NOWAIT probe would also prove absence-of-lock, but this exercises the
// actual hazard lockTaskRowIfStepless exists to prevent (a concurrent
// writer completing unserialized against the caller's own mutation), and
// doubles as proof that a genuinely unrelated write (not a reattachment)
// is also correctly held back, not just rejected outright.
func assertTaskRowBlocksConcurrentWrite(t *testing.T, ctx context.Context, repo *Repository, tx *sqlx.Tx, taskID string) {
	t.Helper()
	writeDone := make(chan error, 1)
	go func() {
		_, err := repo.db.ExecContext(ctx, repo.db.Rebind(
			`UPDATE tasks SET title = ? WHERE id = ?`,
		), "concurrent write", taskID)
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		t.Fatalf("a concurrent write to the task's own row should have blocked (no protective lock held) "+
			"while the caller's tx is still open, got: %v", err)
	case <-time.After(300 * time.Millisecond):
		// expected: still blocked
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("concurrent write after commit: %v", err)
	}
}
