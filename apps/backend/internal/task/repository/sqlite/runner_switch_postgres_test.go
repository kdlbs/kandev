package sqlite

// Postgres proof that SwitchTaskRunner's row lock (internal/db.LockTaskRowInTx,
// called with r.db.DriverName()) is a genuine PostgreSQL `FOR UPDATE` wait, not
// merely correct by construction on SQLite's single-writer-connection no-op.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs this in postgres-boot.
//
// runner_switch_test.go's concurrency tests (e.g.
// TestSwitchTaskRunner_ConcurrentExecutorRunningWriteNeverBlendsWithSwitch) all
// run against SQLite, where LockTaskRowInTx is a deliberate no-op relying on
// the writer pool's MaxOpenConns(1) — they prove SwitchTaskRunner's own
// sequencing but never exercise the `dialect.IsPostgres` branch or the real
// `SELECT ... FOR UPDATE` this feature adds to internal/db/tasklock.go. This
// test mirrors the holder/mover/observer shape in
// task_update_if_workflow_matches_postgres_test.go and
// turn_step_stamp_postgres_test.go (openSecondPostgresConnection,
// waitForPostgresLock): one goroutine holds the task row's lock open via the
// same internal/db.LockTaskRowInTx production helper on an independent
// connection, and a second, independent connection runs the real
// SwitchTaskRunner. The test observes the mover's backend enter a genuine
// PostgreSQL lock wait before releasing the holder, so a scheduler delay can't
// masquerade as contention, then asserts the switch applies only after release
// and the holder's own row read is unaffected by the switch's later write.

import (
	"context"
	"sync"
	"testing"
	"time"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresSwitchTaskRunnerBlocksOnConcurrentRowLock(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-runner-switch", Name: "PG Runner Switch Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-pg-runner-switch", WorkspaceID: "ws-pg-runner-switch", Name: "pg-runner-switch-repo"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	task := &models.Task{
		ID: "task-pg-runner-switch", WorkspaceID: "ws-pg-runner-switch", Title: "PG Runner Switch Task",
		Priority: "medium",
		Metadata: map[string]interface{}{models.MetaKeyExecutorProfileID: "profile-old"},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-pg-runner-switch", TaskID: task.ID, RepositoryID: "repo-pg-runner-switch", BaseBranch: "main",
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}

	// Independent connections, not repo's single isolated connection: see
	// openSecondPostgresConnection's doc comment for why a shared connection
	// would serialize the calls at Go's pool rather than at the PostgreSQL
	// server, proving nothing about the FOR UPDATE clause itself. The mover
	// needs its own writer AND reader connection: SwitchTaskRunner holds an
	// open r.db transaction while reading other tables via r.ro, and a
	// shared single connection between the two would self-deadlock under
	// MaxOpenConns(1) exactly as runner_switch_test.go's doc comment
	// describes for SQLite.
	holderDB := openSecondPostgresConnection(t, dsn, db)
	moverWriterDB := openSecondPostgresConnection(t, dsn, db)
	moverReaderDB := openSecondPostgresConnection(t, dsn, db)
	observerDB := openSecondPostgresConnection(t, dsn, db)
	moverRepo, err := NewWithDB(moverWriterDB, moverReaderDB, nil)
	if err != nil {
		t.Fatalf("init postgres schema on mover connection: %v", err)
	}

	// The holder plays a concurrent class-2 writer (e.g. UpsertExecutorRunning)
	// mid-transaction: it acquires the row's FOR UPDATE lock via the exact
	// production helper SwitchTaskRunner itself calls, then holds the
	// transaction open — uncommitted — until this test explicitly releases it.
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
	if err := moverWriterDB.Get(&moverPID, "SELECT pg_backend_pid()"); err != nil {
		t.Fatalf("read mover backend pid: %v", err)
	}

	// The mover runs the unmodified production SwitchTaskRunner on the
	// second, genuinely independent connection pair, exactly as the real
	// task.runner action does.
	moverDone := make(chan *models.RunnerSwitchResult, 1)
	moverErr := make(chan error, 1)
	moverFinished := make(chan struct{})
	go func() {
		defer close(moverFinished)
		result, err := moverRepo.SwitchTaskRunner(ctx, models.RunnerSwitchRequest{
			TaskID:            task.ID,
			ExecutorProfileID: "profile-new",
		})
		moverDone <- result
		moverErr <- err
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
		t.Fatal("SwitchTaskRunner returned while the holder still held the row's FOR UPDATE lock — LockTaskRowInTx is not genuinely serialized against a concurrent PostgreSQL writer")
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
			t.Fatalf("SwitchTaskRunner (after lock release) error = %v, want nil", err)
		}
		result := <-moverDone
		if !result.Changed {
			t.Fatalf("result.Changed = false, want true")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SwitchTaskRunner to proceed after the lock was released")
	}

	final, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got := final.Metadata[models.MetaKeyExecutorProfileID]; got != "profile-new" {
		t.Fatalf("persisted executor_profile_id = %v, want profile-new (the switch must apply once the concurrent PostgreSQL lock is released)", got)
	}
}
