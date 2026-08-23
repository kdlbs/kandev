package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace is
// the AC-46/48 PostgreSQL counterpart to
// TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot: it
// exercises the same workspace-row lock the CAS variant reuses, proving that
// with several real connections racing the same expectedStepID
// precondition, exactly one attempt observes a match and applies.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace(t *testing.T) {
	const (
		concurrency = 8
		sourceStep  = "postgres-cas-source"
		targetStep  = "postgres-cas-target"
	)
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), concurrency)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "postgres-cas-workspace", Name: "Postgres CAS workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	for i, stepID := range []string{sourceStep, targetStep} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO workflow_steps (id, workflow_id, name, position)
			VALUES (?, ?, ?, ?)
		`), stepID, "postgres-cas-workflow", stepID, i); err != nil {
			t.Fatalf("seed workflow step %s: %v", stepID, err)
		}
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "postgres-cas-task", WorkspaceID: "postgres-cas-workspace", WorkflowID: "postgres-cas-workflow",
		WorkflowStepID: sourceStep, Title: "CAS race candidate", WIPAdmitted: true,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	start := make(chan struct{})
	results := make(chan struct {
		applied bool
		err     error
	}, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			own, err := repo.GetTask(ctx, "postgres-cas-task")
			if err != nil {
				results <- struct {
					applied bool
					err     error
				}{false, err}
				return
			}
			applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, own, sourceStep, targetStep, 0)
			results <- struct {
				applied bool
				err     error
			}{applied, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	appliedCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent CAS attempt: %v", result.err)
		}
		if result.applied {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("expected exactly 1 concurrent CAS attempt to apply, got %d", appliedCount)
	}
	final, err := repo.GetTask(ctx, "postgres-cas-task")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowStepID != targetStep {
		t.Fatalf("expected the task to end at %s, got %q", targetStep, final.WorkflowStepID)
	}
}

// TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_PreconditionReadLocksRow
// is the regression coverage for the lost-update race the CAS precondition
// check used to be exposed to: before the fix, the expectedStepID check in
// updateTaskWithWorkflowStepAdmission read the task's current step with a
// bare, lock-free SELECT, so a concurrent plain UpdateTask could commit a
// step change in the window between that read and the CAS transaction's own
// write, and the CAS write would silently clobber it. The fix makes the
// precondition read go through readTaskStepInTx, which takes `FOR UPDATE` on
// Postgres — the same lock every other task-row writer in this file already
// takes before mutating workflow_step_id.
//
// TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace above
// only proves two CAS attempts serialize against each other; it does not
// exercise CAS-vs-plain-UpdateTask, which is the exact interleaving this test
// targets. It does so directly: hold the row's FOR UPDATE lock via the same
// query readTaskStepInTx issues, prove a concurrent UpdateTask call blocks
// until the lock is released, then prove it observes the up-to-date step once
// it acquires the lock. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_PreconditionReadLocksRow(t *testing.T) {
	const (
		sourceStep = "postgres-cas-lock-source"
		otherStep  = "postgres-cas-lock-other"
	)
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "postgres-cas-lock-workspace", Name: "Postgres CAS lock workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	for i, stepID := range []string{sourceStep, otherStep} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO workflow_steps (id, workflow_id, name, position)
			VALUES (?, ?, ?, ?)
		`), stepID, "postgres-cas-lock-workflow", stepID, i); err != nil {
			t.Fatalf("seed workflow step %s: %v", stepID, err)
		}
	}
	task := &models.Task{
		ID: "postgres-cas-lock-task", WorkspaceID: "postgres-cas-lock-workspace", WorkflowID: "postgres-cas-lock-workflow",
		WorkflowStepID: sourceStep, Title: "CAS lock candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// Hold the same row lock readTaskStepInTx takes, simulating a CAS
	// attempt paused between its precondition read and its commit.
	holder, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder tx: %v", err)
	}
	var wf, step string
	if err := holder.QueryRowContext(ctx, db.Rebind(
		`SELECT workflow_id, workflow_step_id FROM tasks WHERE id = ? FOR UPDATE`), task.ID,
	).Scan(&wf, &step); err != nil {
		t.Fatalf("holder lock read: %v", err)
	}

	updateDone := make(chan error, 1)
	go func() {
		moved := &models.Task{ID: task.ID, WorkspaceID: task.WorkspaceID, WorkflowID: task.WorkflowID,
			WorkflowStepID: otherStep, Title: task.Title, Priority: task.Priority, WIPAdmitted: true}
		updateDone <- repo.UpdateTask(ctx, moved)
	}()

	select {
	case err := <-updateDone:
		t.Fatalf("expected concurrent UpdateTask to block while the CAS-style FOR UPDATE lock is held, but it completed (err=%v)", err)
	case <-time.After(200 * time.Millisecond):
		// Still blocked, as expected.
	}

	if err := holder.Commit(); err != nil {
		t.Fatalf("commit holder tx: %v", err)
	}

	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("UpdateTask after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected the blocked UpdateTask to complete once the holder lock was released")
	}

	final, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowStepID != otherStep {
		t.Fatalf("expected the task to end at %s once UpdateTask acquired the lock, got %q", otherStep, final.WorkflowStepID)
	}
}
