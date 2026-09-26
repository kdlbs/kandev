package sqlite

// Postgres parity for the ledger writer: genesis row, a move via UpdateTask,
// AddTaskToWorkflow/RemoveTaskFromWorkflow, and the chain invariant. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresLedgerWriterGenesisMoveAttachDetach(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerManualMove, ActorKind: steptelemetry.ActorHuman, ActorID: "user-pg",
	})
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-pg-ledger", Name: "PG Ledger Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	task := &models.Task{ID: "task-pg-ledger", WorkspaceID: "ws-pg-ledger", WorkflowID: "wf-pg", WorkflowStepID: "step-a", Title: "PG Task", Priority: "medium"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task.WorkflowStepID = "step-b"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if err := repo.RemoveTaskFromWorkflow(ctx, task.ID, "wf-pg"); err != nil {
		t.Fatalf("RemoveTaskFromWorkflow: %v", err)
	}
	if err := repo.AddTaskToWorkflow(ctx, task.ID, "wf-pg-2", "step-c", 0); err != nil {
		t.Fatalf("AddTaskToWorkflow: %v", err)
	}

	rows := stepTransitionRowsForTask(t, repo, task.ID)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4 (genesis, move, detach, attach)", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		prevTo := ""
		if rows[i-1].toWorkflowStepID != nil {
			prevTo = *rows[i-1].toWorkflowStepID
		}
		curFrom := ""
		if rows[i].fromWorkflowStepID != nil {
			curFrom = *rows[i].fromWorkflowStepID
		}
		if prevTo != curFrom {
			t.Fatalf("chain broken at row %d: prev to=%q, this from=%q", i, prevTo, curFrom)
		}
	}
	last := rows[len(rows)-1]
	if last.toWorkflowStepID == nil || *last.toWorkflowStepID != "step-c" {
		t.Fatalf("final to_workflow_step_id = %v, want step-c", last.toWorkflowStepID)
	}
}

// TestPostgresLedgerWriterConcurrentMovesProduceIntactChain is the Postgres
// counterpart of TestChainConcurrentMovesProduceTwoRowsWithIntactChain
// (step_transitions_chain_test.go), barrier-controlled rather than relying on
// goroutine-scheduling luck to exercise readTaskStepInTx's real Postgres
// "FOR UPDATE" lock contention: both movers block on the same gate so they
// race into the lock as close to simultaneously as genuinely concurrent
// callers would. This is the scenario that caught a real bug during this
// PR's review — occurred_at was stamped before the transactional lock, so
// two racing movers could commit in one order but record timestamps in the
// other, breaking the (occurred_at, id) chain invariant only under real
// concurrent Postgres connections (SQLite's single-writer pool serializes
// regardless, which is why no SQLite test could have caught it).
//
// The two movers deliberately run on two independent *Repository instances
// backed by two independent physical connections (repo/db and moverRepo/
// moverDB, both pinned to the same isolated schema — see
// openSecondPostgresConnection in turn_step_stamp_postgres_test.go).
// testutil.OpenIsolatedPostgres caps db at one physical connection, so if
// both UpdateTask calls below shared it, Go's connection pool alone would
// serialize them — the two calls would never reach the Postgres server at
// the same time, and this test would pass whether or not
// readTaskStepInTx's FOR UPDATE clause exists at all. Two connections make
// the calls genuinely race at the server, which is the only way this test
// can distinguish "the lock serializes movers" from "the lock is dead
// code": reintroducing the historical bug this test guards against (stamp
// occurred_at before acquiring the lock) breaks the chain here but is
// invisible to a single-shared-connection version of the same test, which
// is exactly what made the original bug undetectable before this PR.
func TestPostgresLedgerWriterConcurrentMovesProduceIntactChain(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	moverDB := openSecondPostgresConnection(t, dsn, db)
	moverRepo, err := NewWithDB(moverDB, moverDB, nil)
	if err != nil {
		t.Fatalf("init postgres schema on second connection: %v", err)
	}
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerManualMove, ActorKind: steptelemetry.ActorHuman, ActorID: "user-pg-concurrent",
	})
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-pg-concurrent", Name: "PG Concurrent Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	task := &models.Task{ID: "task-pg-concurrent", WorkspaceID: "ws-pg-concurrent", WorkflowID: "wf-pg-concurrent", WorkflowStepID: "step-a", Title: "PG Concurrent Task", Priority: "medium"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	var start sync.WaitGroup
	start.Add(1)
	var g errgroup.Group
	g.Go(func() error {
		start.Wait()
		moved := *task
		moved.WorkflowStepID = "step-b"
		return repo.UpdateTask(ctx, &moved)
	})
	g.Go(func() error {
		start.Wait()
		moved := *task
		moved.WorkflowStepID = "step-c"
		return moverRepo.UpdateTask(ctx, &moved)
	})
	start.Done()
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent UpdateTask: %v", err)
	}

	rows := assertChainIntact(t, repo, task.ID)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (genesis + 2 concurrent moves)", len(rows))
	}

	final, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowStepID != "step-b" && final.WorkflowStepID != "step-c" {
		t.Fatalf("final workflow_step_id = %q, want step-b or step-c (last committed writer wins)", final.WorkflowStepID)
	}
}

// TestPostgresReadTaskStepInTxBlocksOnConcurrentRowLock is the deterministic
// counterpart to TestPostgresLedgerWriterConcurrentMovesProduceIntactChain
// above. That test's sync.WaitGroup barrier only releases both goroutines at
// the same instant — it does not force them to overlap once released, so
// the Go scheduler can (and often does) run one mover's entire transaction
// to completion before the other even begins. When that happens the second
// mover reads an already-updated step and the chain looks intact whether or
// not readTaskStepInTx's FOR UPDATE clause (step_transitions.go:28-30) does
// anything at all. Reintroducing the historical stamp-before-lock bug that
// test guards against produced only 7 failures out of 20 runs — a coin
// flip, not a control.
//
// This test instead proves the lock's actual serialization property and the
// timestamp placement directly and deterministically, mirroring the pattern
// in turn_step_stamp_postgres_test.go's
// TestPostgresCreateTurnWithStepStampBlocksOnConcurrentStepMove (same
// openSecondPostgresConnection helper): one goroutine acquires the row's
// FOR UPDATE lock via readTaskStepInTx and holds its transaction open on a
// channel; a second, independent connection then attempts a real production
// UpdateTask on the same task. The test observes that backend in PostgreSQL's
// lock-wait state before releasing the holder, so a scheduler delay cannot
// masquerade as contention. Releasing the lock lets the second writer
// proceed, and a fixed repository clock proves its ledger timestamp is
// sampled after the lock is released.
func TestPostgresReadTaskStepInTxBlocksOnConcurrentRowLock(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	// A second, independent connection to the same isolated schema — see
	// openSecondPostgresConnection's doc comment for why a shared
	// db.OpenIsolatedPostgres connection (capped at one physical connection)
	// cannot exercise genuine Postgres-level lock contention.
	moverDB := openSecondPostgresConnection(t, dsn, db)
	observerDB := openSecondPostgresConnection(t, dsn, db)
	moverRepo, err := NewWithDB(moverDB, moverDB, nil)
	if err != nil {
		t.Fatalf("init postgres schema on second connection: %v", err)
	}
	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerManualMove, ActorKind: steptelemetry.ActorHuman, ActorID: "user-pg-lock",
	})
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-pg-lock", Name: "PG Lock Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	task := &models.Task{ID: "task-pg-lock", WorkspaceID: "ws-pg-lock", WorkflowID: "wf-pg-lock", WorkflowStepID: "step-a", Title: "PG Lock Task", Priority: "medium"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// The holder goroutine plays the concurrent transaction: it acquires the
	// same readTaskStepInTx FOR UPDATE lock a real step move takes (on the
	// first connection), then holds its transaction open — without writing
	// anything — until this test explicitly releases it. It never changes
	// workflow_step_id, so the mover's later UpdateTask below is the only
	// real step move in this test.
	lockHeld := make(chan struct{})
	releaseLock := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseLock) }) }
	holderFinished := make(chan struct{})
	var holderErr error
	go func() {
		defer close(holderFinished)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			holderErr = err
			return
		}
		if _, _, _, err := repo.readTaskStepInTx(ctx, tx, task.ID); err != nil {
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
	const moverStampYear = 2091
	moverStamp := time.Date(moverStampYear, time.January, 2, 3, 4, 5, 123456000, time.UTC)
	stampUsed := make(chan struct{})
	var stampOnce sync.Once
	moverRepo.clockNow = func() time.Time {
		stampOnce.Do(func() { close(stampUsed) })
		return moverStamp
	}

	// The mover runs on moverRepo/moverDB, the SECOND connection — genuinely
	// independent of db, so its call below contends for the real Postgres
	// row lock rather than for Go's connection pool. It is the unmodified
	// production UpdateTask path, not a simulation.
	moverDone := make(chan error, 1)
	moverFinished := make(chan struct{})
	go func() {
		defer close(moverFinished)
		moved := *task
		moved.WorkflowStepID = "step-b"
		moverDone <- moverRepo.UpdateTask(ctx, &moved)
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
		t.Fatal("moverRepo.UpdateTask returned while a concurrent transaction still held the row's FOR UPDATE lock — readTaskStepInTx is not serializing concurrent movers")
	default:
	}
	select {
	case <-stampUsed:
		t.Fatal("mover repository clock was sampled before the row lock was released")
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
		if err != nil {
			t.Fatalf("moverRepo.UpdateTask: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for moverRepo.UpdateTask to proceed after the lock was released")
	}
	select {
	case <-stampUsed:
	case <-time.After(5 * time.Second):
		t.Fatal("mover repository clock was not sampled after the row lock was released")
	}

	final, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowStepID != "step-b" {
		t.Fatalf("final workflow_step_id = %q, want step-b (the mover's UpdateTask committed after the holder released its lock)", final.WorkflowStepID)
	}

	rows := assertChainIntact(t, repo, task.ID)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (genesis + the mover's real UpdateTask move; the holder never wrote anything)", len(rows))
	}
	last := rows[len(rows)-1]
	if last.toWorkflowStepID == nil || *last.toWorkflowStepID != "step-b" {
		t.Fatalf("final to_workflow_step_id = %v, want step-b", last.toWorkflowStepID)
	}
	if !last.occurredAt.Equal(moverStamp) {
		t.Fatalf("final occurred_at = %v, want the post-lock repository timestamp %v", last.occurredAt, moverStamp)
	}
}
