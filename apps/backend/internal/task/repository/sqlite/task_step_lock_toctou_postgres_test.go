package sqlite

// TestLockTaskStepForWriteLocksCurrentStepAfterConcurrentMove proves
// lockTaskStepForWrite (used by ArchiveTask, ArchiveTaskIfActive, DeleteTask
// and UnarchiveTask/UnarchiveTaskByCascade) still locks the task's true
// current step when a concurrent move changes that step in the gap between
// its candidate read and its lock acquisition. Before this fix,
// lockTaskStepForWrite read the task's step once, unlocked, and only then
// called lockWorkflowStepForWrite on whatever it read - so a move landing in
// that gap left it locking a step the task had already left, defeating its
// own serialization guarantee against a reorder of the task's real step.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestLockTaskStepForWriteLocksCurrentStepAfterConcurrentMove(t *testing.T) {
	const stepA = "toctou-step-a"
	const stepB = "toctou-step-b"
	repo := seedVisibilityLockFixture(t, "toctou-ws", "toctou-workflow", stepA)
	ctx := context.Background()

	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?)
	`), stepB, "toctou-workflow", stepB, 1); err != nil {
		t.Fatalf("seed step B: %v", err)
	}

	task := &models.Task{
		ID: "toctou-task", WorkspaceID: "toctou-ws", WorkflowID: "toctou-workflow",
		WorkflowStepID: stepA, Title: "Moving", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	paused := make(chan struct{})
	release := make(chan struct{})
	var pauseOnce sync.Once
	var fired bool
	repo.taskStepLockBeforeAcquireHook = func(candidateStepID string) {
		if fired {
			return
		}
		fired = true
		if candidateStepID != stepA {
			t.Errorf("first candidate step = %q, want %q", candidateStepID, stepA)
		}
		pauseOnce.Do(func() { close(paused) })
		<-release
	}
	var releaseOnce sync.Once
	releaseFn := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFn)
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

	<-paused // lockTaskStepForWrite read stepA, has not locked it yet

	// A concurrent move commits fully in the exact gap between the read
	// above and the lock it is about to take.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`,
	), stepB, task.ID); err != nil {
		t.Fatalf("simulate concurrent move: %v", err)
	}

	releaseFn()
	if err := <-lockDone; err != nil {
		t.Fatalf("lockTaskStepForWrite: %v", err)
	}

	// tx must now hold stepB's row lock: a second connection trying to lock
	// it should block until tx ends. repo.db's pool has spare connections
	// (seedVisibilityLockFixture opens it with maxConns=3), so this reaches
	// the same isolated schema over a different physical connection.
	blocked := make(chan struct{})
	go func() {
		var id string
		_ = repo.db.QueryRowContext(ctx, repo.db.Rebind(
			`SELECT id FROM workflow_steps WHERE id = ? FOR UPDATE`,
		), stepB).Scan(&id)
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatalf("a second connection locked step %q while lockTaskStepForWrite's transaction should still "+
			"hold it — it locked the task's stale pre-move step instead of its current one", stepB)
	case <-time.After(200 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatalf("second connection never acquired step %q's lock after commit", stepB)
	}
}
