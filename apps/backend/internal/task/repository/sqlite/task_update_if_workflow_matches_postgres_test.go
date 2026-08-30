package sqlite

// Postgres proof that UpdateTaskIfWorkflowMatches's CAS check is genuinely
// serialized against a concurrent reassignment, not merely usually-correct.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs this in postgres-boot.
//
// task_update_if_workflow_matches_test.go injects its race sequentially (move
// the task, then call UpdateTaskIfWorkflowMatches) against the SQLite
// repository, which only proves the comparison logic once fromWorkflowID is
// known — it never exercises readTaskStepInTx's real PostgreSQL "FOR UPDATE"
// lock, so it can't tell a genuinely serialized CAS apart from one that reads
// stale data between two racing writers. This test mirrors the pattern in
// step_transitions_writer_postgres_test.go's
// TestPostgresReadTaskStepInTxBlocksOnConcurrentRowLock: one goroutine holds
// the row's FOR UPDATE lock open on an independent connection while
// reassigning the task to a different workflow, and a second, independent
// connection calls the real UpdateTaskIfWorkflowMatches with the task's
// pre-reassignment workflow id as expectedWorkflowID. The test observes the
// mover enter a genuine PostgreSQL lock wait before releasing the holder, so
// a scheduler delay can't masquerade as contention, then asserts the mover
// gets ErrWorkflowResolutionConflict and the holder's reassignment survives
// untouched.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresUpdateTaskIfWorkflowMatchesBlocksOnConcurrentReassignment(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	// Independent connections, not repo's single isolated connection: see
	// openSecondPostgresConnection's doc comment for why a shared connection
	// would serialize the calls at Go's pool rather than at the Postgres
	// server, proving nothing about the FOR UPDATE clause itself.
	holderDB := openSecondPostgresConnection(t, dsn, db)
	moverDB := openSecondPostgresConnection(t, dsn, db)
	observerDB := openSecondPostgresConnection(t, dsn, db)
	moverRepo, err := NewWithDB(moverDB, moverDB, nil)
	if err != nil {
		t.Fatalf("init postgres schema on second connection: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-cas", Name: "PG CAS Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	task := &models.Task{
		ID: "task-pg-cas", WorkspaceID: "ws-pg-cas", WorkflowID: "workflow-a",
		WorkflowStepID: "step-source", Title: "PG CAS Task", Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// The holder plays the concurrent legitimate reassignment: it acquires
	// the row's FOR UPDATE lock via readTaskStepInTx (on its own connection),
	// reassigns the task to workflow-b, and holds the transaction open —
	// uncommitted — until this test explicitly releases it.
	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseLock) }) }
	holderFinished := make(chan struct{})
	var holderErr error
	go func() {
		defer close(holderFinished)
		tx, err := holderDB.BeginTx(ctx, nil)
		if err != nil {
			holderErr = err
			return
		}
		if _, _, _, err := repo.readTaskStepInTx(ctx, tx, task.ID); err != nil {
			_ = tx.Rollback()
			holderErr = err
			return
		}
		if _, err := tx.ExecContext(ctx, holderDB.Rebind(
			`UPDATE tasks SET workflow_id = ?, workflow_step_id = ? WHERE id = ?`),
			"workflow-b", "step-destination", task.ID); err != nil {
			_ = tx.Rollback()
			holderErr = err
			return
		}
		close(lockHeld)
		<-releaseLock
		holderErr = tx.Commit()
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-holderFinished:
			if holderErr != nil {
				t.Errorf("holder goroutine during cleanup: %v", holderErr)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for holder goroutine during cleanup")
		}
	})

	select {
	case <-lockHeld:
	case <-holderFinished:
		t.Fatalf("holder goroutine failed before acquiring the row lock: %v", holderErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the holder goroutine to acquire the row lock")
	}

	var moverPID int
	if err := moverDB.Get(&moverPID, "SELECT pg_backend_pid()"); err != nil {
		t.Fatalf("read mover backend pid: %v", err)
	}

	// The mover runs on moverRepo/moverDB, the SECOND connection — genuinely
	// independent of holderDB, so its call below contends for the real
	// PostgreSQL row lock rather than for Go's connection pool. It is the
	// unmodified production UpdateTaskIfWorkflowMatches path, called with the
	// task's pre-reassignment workflow id, exactly as a plugin move's
	// same-step CAS guard does.
	moved := *task
	moved.WorkflowStepID = "step-source-mover"
	moverDone := make(chan error, 1)
	moverFinished := make(chan struct{})
	go func() {
		defer close(moverFinished)
		moverDone <- moverRepo.UpdateTaskIfWorkflowMatches(ctx, &moved, "workflow-a")
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-moverFinished:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for mover goroutine during cleanup")
		}
	})

	if err := waitForPostgresLock(ctx, observerDB, moverPID, moverFinished); err != nil {
		t.Fatal(err)
	}
	select {
	case <-moverDone:
		t.Fatal("moverRepo.UpdateTaskIfWorkflowMatches returned while the holder's reassignment still held the row's FOR UPDATE lock — the CAS check is not serialized against a concurrent PostgreSQL writer")
	default:
	}

	release()
	select {
	case <-holderFinished:
		if holderErr != nil {
			t.Fatalf("holder goroutine: %v", holderErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for holder goroutine after releasing the row lock")
	}

	select {
	case err := <-moverDone:
		if err == nil {
			t.Fatal("moverRepo.UpdateTaskIfWorkflowMatches: got nil error, want ErrWorkflowResolutionConflict (the holder reassigned the task to workflow-b while this call's expectedWorkflowID was workflow-a)")
		}
		if !errors.Is(err, ErrWorkflowResolutionConflict) {
			t.Fatalf("moverRepo.UpdateTaskIfWorkflowMatches: err = %v, want errors.Is(err, ErrWorkflowResolutionConflict)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for moverRepo.UpdateTaskIfWorkflowMatches to proceed after the lock was released")
	}

	final, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowID != "workflow-b" || final.WorkflowStepID != "step-destination" {
		t.Fatalf("final task = {workflow_id: %q, workflow_step_id: %q}, want {workflow-b, step-destination} (the holder's committed reassignment must survive the mover's rejected CAS)",
			final.WorkflowID, final.WorkflowStepID)
	}
}
