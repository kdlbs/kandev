package sqlite_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresCancelRunsWhere_DoesNotOverwriteAConcurrentlyFinishedRun is
// the two-connection regression for a TOCTOU CancelRunsWhere used to have:
// its cancellable-status guard was applied only to the candidate read, not
// to the write, so a run that genuinely finished on another connection
// between the two could be silently overwritten to cancelled. Real
// Postgres row locking forces the interleaving this needs: one connection
// holds an uncommitted finish on the row, CancelRunsWhere's write blocks on
// that row lock, and only proceeds once the finish commits — at which
// point it must see the fresh 'finished' status and cancel nothing. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresCancelRunsWhere_DoesNotOverwriteAConcurrentlyFinishedRun(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	// db is the repo's own pool: OpenIsolatedPostgres pins it to exactly
	// one physical connection and sets that connection's search_path to a
	// fresh schema, so every statement through it (including the schema
	// init below and repo.CancelRunsWhere's own transaction) lands there.
	db := testutil.OpenIsolatedPostgres(t, dsn)

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	repo := officeRepo.RunsRepository()
	ctx := context.Background()

	// finisherDB is a second, genuinely separate physical connection to
	// the SAME schema. A pool-wide MaxOpenConns bump on a single *sqlx.DB
	// would not do this safely: search_path is a per-connection session
	// setting, so a new connection the pool opens under load would not
	// inherit it and could silently operate against the wrong schema.
	var schema string
	if err := db.Get(&schema, "SELECT current_schema()"); err != nil {
		t.Fatalf("read isolated schema name: %v", err)
	}
	finisherDB, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open finisher connection: %v", err)
	}
	finisherDB.SetMaxOpenConns(1)
	finisherDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = finisherDB.Close() })
	if _, err := finisherDB.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("set finisher search_path: %v", err)
	}

	run := &models.Run{AgentProfileID: "a1", Reason: "task_assigned", Payload: "{}", Status: "queued"}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, db.Rebind(
		`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`,
	), now, run.ID); err != nil {
		t.Fatalf("seed claimed: %v", err)
	}

	tx, err := finisherDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin finisher tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, finisherDB.Rebind(
		`UPDATE runs SET status = 'finished', outcome = 'processed', finished_at = ? WHERE id = ?`,
	), now, run.ID); err != nil {
		t.Fatalf("finisher update (holds the row lock, uncommitted): %v", err)
	}

	var (
		wg         sync.WaitGroup
		cancelled  []runssqlite.CancelledRun
		cancelErr  error
		cancelDone = make(chan struct{})
		commitErr  error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(cancelDone)
		cancelled, cancelErr = repo.CancelRunsWhere(ctx, "too late", `id = ?`, run.ID)
	}()

	// Give CancelRunsWhere's write time to reach Postgres and block on the
	// row lock the finisher's uncommitted transaction holds, before that
	// transaction commits. Without this window the two goroutines could
	// run in either order and the test would not exercise the lock-wait
	// path this bug depends on.
	select {
	case <-cancelDone:
		t.Fatal("CancelRunsWhere returned before the finisher committed; it never blocked on the row lock")
	case <-time.After(200 * time.Millisecond):
	}

	commitErr = tx.Commit()
	wg.Wait()
	if commitErr != nil {
		t.Fatalf("finisher commit: %v", commitErr)
	}
	if cancelErr != nil {
		t.Fatalf("cancel where: %v", cancelErr)
	}

	if len(cancelled) != 0 {
		t.Fatalf("cancelled = %d, want 0: a run that committed finished must not be reported cancelled", len(cancelled))
	}
	if got := mustGetRunStatus(t, db, run.ID); got != "finished" {
		t.Fatalf("run status = %q, want finished (the concurrent cancel must not have overwritten it)", got)
	}
}

func mustGetRunStatus(t *testing.T, db *sqlx.DB, id string) string {
	t.Helper()
	var status string
	if err := db.Get(&status, db.Rebind(`SELECT status FROM runs WHERE id = ?`), id); err != nil {
		t.Fatalf("get run status: %v", err)
	}
	return status
}
