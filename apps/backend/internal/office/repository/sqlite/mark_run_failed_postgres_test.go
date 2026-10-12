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
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresMarkRunFailed_DoesNotOverwriteAConcurrentlyCancelledRun is
// the two-connection regression for Review round 3, R3-1: MarkRunFailed's
// write used to carry no status guard at all, so a run cancelled on
// another connection between HandleAgentFailure's read and this write
// would have its 'cancelled' status and real finished_at silently
// overwritten to 'failed'. Real Postgres row locking forces the
// interleaving this needs: one connection holds an uncommitted cancel on
// the row, MarkRunFailed's write blocks on that row lock, and only
// proceeds once the cancel commits — at which point the guard
// (status = 'claimed') must see the fresh 'cancelled' status and write
// nothing. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresMarkRunFailed_DoesNotOverwriteAConcurrentlyCancelledRun(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	ctx := context.Background()

	var schema string
	if err := db.Get(&schema, "SELECT current_schema()"); err != nil {
		t.Fatalf("read isolated schema name: %v", err)
	}
	cancellerDB, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open canceller connection: %v", err)
	}
	cancellerDB.SetMaxOpenConns(1)
	cancellerDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = cancellerDB.Close() })
	if _, err := cancellerDB.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("set canceller search_path: %v", err)
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

	tx, err := cancellerDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin canceller tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, cancellerDB.Rebind(
		`UPDATE runs SET status = 'cancelled', cancel_reason = ?, finished_at = ? WHERE id = ?`,
	), "too late", now, run.ID); err != nil {
		t.Fatalf("canceller update (holds the row lock, uncommitted): %v", err)
	}

	var (
		wg        sync.WaitGroup
		wrote     bool
		markErr   error
		markDone  = make(chan struct{})
		commitErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(markDone)
		wrote, markErr = repo.MarkRunFailed(ctx, run.ID, "provider exploded")
	}()

	// Give MarkRunFailed's write time to reach Postgres and block on the
	// row lock the canceller's uncommitted transaction holds, before that
	// transaction commits. Without this window the two goroutines could
	// run in either order and the test would not exercise the lock-wait
	// path this bug depends on.
	select {
	case <-markDone:
		t.Fatal("MarkRunFailed returned before the canceller committed; it never blocked on the row lock")
	case <-time.After(200 * time.Millisecond):
	}

	commitErr = tx.Commit()
	wg.Wait()
	if commitErr != nil {
		t.Fatalf("canceller commit: %v", commitErr)
	}
	if markErr != nil {
		t.Fatalf("mark run failed: %v", markErr)
	}

	if wrote {
		t.Fatal("wrote = true, want false: a run that committed cancelled must not be reported failed")
	}
	if got := mustGetRunStatus(t, db, run.ID); got != "cancelled" {
		t.Fatalf("run status = %q, want cancelled (the concurrent fail must not have overwritten it)", got)
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
