package sqlite

// Postgres proof that UpsertExecutorRunning's own row-lock retrofit
// (internal/db.LockTaskRowInTx, called with r.db.DriverName() — see the doc
// comment on UpsertExecutorRunning in executor.go) is a genuine PostgreSQL
// `FOR UPDATE` wait, not merely correct by construction on SQLite's
// single-writer-connection no-op. Skips unless KANDEV_TEST_POSTGRES_DSN is
// set; CI runs this in postgres-boot.
//
// runner_switch_postgres_test.go already proves this for SwitchTaskRunner
// itself, with a holder that plays a concurrent class-2 writer. This test
// proves the other side of that same serialization guarantee: it holds the
// row lock with the same raw LockTaskRowInTx helper and runs the
// unmodified, production UpsertExecutorRunning as the mover, so a class-2
// write's own lock acquisition — not just SwitchTaskRunner's — is verified
// against a real PostgreSQL FOR UPDATE wait. UpsertExecutorRunning has no
// injection point to pause mid-transaction (BeginTxx, lock, write, and the
// caller's own commit all happen inside one call), so it cannot play the
// holder role the way SwitchTaskRunner's sibling test does; it can only be
// the mover here.

import (
	"context"
	"sync"
	"testing"
	"time"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresUpsertExecutorRunningBlocksOnConcurrentRowLock(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-exec-running-lock", Name: "PG Executor Running Lock Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	task := &models.Task{
		ID: "task-pg-exec-running-lock", WorkspaceID: "ws-pg-exec-running-lock", Title: "PG Executor Running Lock Task",
		Priority: "medium",
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Independent connections, not repo's single isolated connection: see
	// openSecondPostgresConnection's doc comment for why a shared connection
	// would serialize the calls at Go's pool rather than at the PostgreSQL
	// server, proving nothing about the FOR UPDATE clause itself.
	holderDB := openSecondPostgresConnection(t, dsn, db)
	moverDB := openSecondPostgresConnection(t, dsn, db)
	observerDB := openSecondPostgresConnection(t, dsn, db)
	moverRepo, err := NewWithDB(moverDB, moverDB, nil)
	if err != nil {
		t.Fatalf("init postgres schema on mover connection: %v", err)
	}

	// The holder plays a concurrent runner switch mid-transaction: it
	// acquires the row's FOR UPDATE lock via the exact production helper
	// UpsertExecutorRunning itself calls, then holds the transaction
	// open — uncommitted — until this test explicitly releases it.
	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseLock) }) }
	holderFinished := make(chan struct{})
	var holderErr error
	go func() {
		defer close(holderFinished)
		tx, err := holderDB.BeginTxx(ctx, nil)
		if err != nil {
			holderErr = err
			return
		}
		if err := kandevdb.LockTaskRowInTx(ctx, tx, holderDB.DriverName(), task.ID); err != nil {
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

	// The mover runs the unmodified production UpsertExecutorRunning on the
	// second, genuinely independent connection, exactly as a real executor
	// launch would.
	moverErr := make(chan error, 1)
	moverFinished := make(chan struct{})
	go func() {
		defer close(moverFinished)
		moverErr <- moverRepo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			SessionID:  "session-pg-exec-running-lock",
			TaskID:     task.ID,
			ExecutorID: "executor-pg-exec-running-lock",
			Status:     "starting",
		})
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
	case <-moverFinished:
		t.Fatal("UpsertExecutorRunning returned while the holder still held the row's FOR UPDATE lock — its LockTaskRowInTx call is not genuinely serialized against a concurrent PostgreSQL writer")
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
	case <-moverFinished:
		if err := <-moverErr; err != nil {
			t.Fatalf("UpsertExecutorRunning (after lock release) error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for UpsertExecutorRunning to proceed after the lock was released")
	}

	rows, err := repo.ListExecutorsRunningByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListExecutorsRunningByTaskID: %v", err)
	}
	if len(rows) != 1 || rows[0].SessionID != "session-pg-exec-running-lock" {
		t.Fatalf("executors_running after release = %#v, want one row for session-pg-exec-running-lock", rows)
	}
}
