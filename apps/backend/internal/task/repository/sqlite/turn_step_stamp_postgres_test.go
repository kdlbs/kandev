package sqlite

// Postgres proof that CreateTurnWithStepStamp's read of the task's current
// step is genuinely serialized against a concurrent step mover, not merely
// usually-correct. Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs
// this in postgres-boot.
//
// A plain "run both concurrently, assert the final stamp is A or B" test
// would pass even on the pre-fix code: both values are individually
// possible outcomes of an unlocked race, so that assertion can't tell the
// fixed code apart from the racy code it replaced. The property that
// actually distinguishes them is serialization: readTaskStepInTx's
// FOR UPDATE lock must make CreateTurnWithStepStamp's read BLOCK until a
// concurrent mover holding the same task row's lock commits or rolls back —
// exactly the guarantee step moves already rely on for the ledger's chain
// invariant (see step_transitions.go).
//
// testutil.OpenIsolatedPostgres deliberately caps its *sqlx.DB at one
// connection (schema isolation via a per-connection search_path), which
// would make this test pass even against the pre-fix racy code: with only
// one physical connection, a second caller's plain unlocked SELECT can't
// even reach the server until the first caller's connection is returned,
// so it "blocks" for the wrong reason and the test would prove nothing.
// This test therefore opens a second, independent connection pinned to the
// same isolated schema so the mover and the turn read genuinely race at
// the Postgres server, not at Go's connection pool.
//
// The mover's transaction holds its lock open on purpose and asserts
// CreateTurnWithStepStamp does not return while it's held, then asserts the
// stamp reflects the mover's committed value once released.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// openSecondPostgresConnection opens an independent *sqlx.DB pinned to the
// same search_path (schema) as db, so a test can exercise genuine
// multi-connection contention against the schema db.OpenIsolatedPostgres
// already isolated and will clean up.
func openSecondPostgresConnection(t *testing.T, dsn string, db *sqlx.DB) *sqlx.DB {
	t.Helper()
	var schema string
	if err := db.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("read current_schema(): %v", err)
	}
	second, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open second postgres connection: %v", err)
	}
	second.SetMaxOpenConns(1)
	second.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = second.Close() })
	if _, err := second.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("set search_path on second connection: %v", err)
	}
	return second
}

func postgresBackendWaitsOnLock(ctx context.Context, db *sqlx.DB, pid int) (bool, error) {
	var waiting bool
	err := db.GetContext(ctx, &waiting, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_stat_activity
			WHERE pid = $1 AND wait_event_type = 'Lock'
		)
	`, pid)
	return waiting, err
}

// waitForPostgresLock waits until the backend identified by pid reports a
// PostgreSQL lock wait. The done channel lets callers fail immediately when
// the operation completes without ever reaching the expected contention.
func waitForPostgresLock(ctx context.Context, observer *sqlx.DB, pid int, done <-chan struct{}) error {
	const timeout = 5 * time.Second
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return fmt.Errorf("operation completed before backend %d entered a PostgreSQL lock wait", pid)
		default:
		}

		waiting, err := postgresBackendWaitsOnLock(deadlineCtx, observer, pid)
		if err != nil {
			return fmt.Errorf("query PostgreSQL lock state for backend %d: %w", pid, err)
		}
		if waiting {
			return nil
		}

		select {
		case <-done:
			return fmt.Errorf("operation completed before backend %d entered a PostgreSQL lock wait", pid)
		case <-ticker.C:
		case <-deadlineCtx.Done():
			return fmt.Errorf("timed out waiting for backend %d to enter a PostgreSQL lock wait", pid)
		}
	}
}

func TestPostgresCreateTurnWithStepStampBlocksOnConcurrentStepMove(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	// A second, independent connection to the same isolated schema — see the
	// file comment for why this is required rather than reusing db.
	moverDB := openSecondPostgresConnection(t, dsn, db)
	observerDB := openSecondPostgresConnection(t, dsn, db)

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-turn-stamp", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-pg-turn-stamp", WorkspaceID: "ws-pg-turn-stamp", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	task := &models.Task{ID: "task-pg-turn-stamp", WorkspaceID: "ws-pg-turn-stamp", WorkflowID: "wf-pg-turn-stamp", WorkflowStepID: "step-a", Title: "PG Turn Stamp Task", Priority: "medium"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-pg-turn-stamp", TaskID: task.ID, State: models.TaskSessionStateRunning,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// Goroutine 1 plays the concurrent mover on the second connection: it
	// acquires the same readTaskStepInTx FOR UPDATE lock a real step move
	// takes, then holds its transaction open (not committing the move to
	// step-b) until this test explicitly releases it.
	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseLock) }) }
	// Guarantees the mover goroutine is released even if a t.Fatal below
	// unwinds this goroutine early (runtime.Goexit still runs deferred
	// funcs) — otherwise a failed assertion leaves the mover blocked
	// forever on <-releaseLock, holding its transaction open, and
	// t.Cleanup's DROP SCHEMA hangs waiting for that lock instead of
	// reporting the actual test failure.
	moverDone := make(chan error, 1)
	moverFinished := make(chan struct{})
	go func() {
		defer close(moverFinished)
		tx, err := moverDB.BeginTx(ctx, nil)
		if err != nil {
			moverDone <- err
			return
		}
		if _, _, _, err := repo.readTaskStepInTx(ctx, tx, task.ID); err != nil {
			_ = tx.Rollback()
			moverDone <- err
			return
		}
		close(lockHeld)
		<-releaseLock
		if _, err := tx.ExecContext(ctx, moverDB.Rebind(`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`), "step-b", task.ID); err != nil {
			_ = tx.Rollback()
			moverDone <- err
			return
		}
		moverDone <- tx.Commit()
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-moverFinished:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for mover goroutine during cleanup")
		}
	})

	select {
	case <-lockHeld:
	case err := <-moverDone:
		t.Fatalf("mover goroutine exited before acquiring the row lock: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the mover goroutine to acquire the row lock")
	}

	turnPID := 0
	if err := db.Get(&turnPID, "SELECT pg_backend_pid()"); err != nil {
		t.Fatalf("read turn backend pid: %v", err)
	}

	// The turn goroutine runs on repo/db, the FIRST connection — genuinely
	// independent of moverDB, so its read below contends for the real
	// Postgres row lock rather than for Go's connection pool.
	turn := &models.Turn{TaskSessionID: "session-pg-turn-stamp", TaskID: task.ID}
	turnDone := make(chan struct{})
	var stamped bool
	var turnErr error
	go func() {
		stamped, turnErr = repo.CreateTurnWithStepStamp(ctx, turn)
		close(turnDone)
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-turnDone:
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for turn goroutine during cleanup")
		}
	})

	if err := waitForPostgresLock(ctx, observerDB, turnPID, turnDone); err != nil {
		t.Fatal(err)
	}
	select {
	case <-turnDone:
		t.Fatal("CreateTurnWithStepStamp returned while the concurrent mover still held the row lock — the step read is not serialized against concurrent movers")
	default:
	}

	release()
	if err := <-moverDone; err != nil {
		t.Fatalf("mover goroutine: %v", err)
	}

	select {
	case <-turnDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for CreateTurnWithStepStamp to proceed after the mover committed")
	}
	if turnErr != nil {
		t.Fatalf("CreateTurnWithStepStamp: %v", turnErr)
	}
	if !stamped {
		t.Fatal("stamped = false, want true")
	}
	if turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] != "step-b" {
		t.Fatalf("stamp = %v, want %q (the mover's committed value — the turn's read was serialized to run after it)",
			turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart], "step-b")
	}
}
