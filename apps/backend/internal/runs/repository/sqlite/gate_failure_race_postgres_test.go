package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresRecordGateOutcome_ConcurrentFirstFailuresBothCount is the
// two-connection regression for the race RecordGateOutcomeTx used to have:
// on PostgreSQL, `SELECT ... FOR UPDATE` locks nothing when the row does
// not exist yet, so two concurrent first-failures for the same
// (workspace, gate) pair could both read "no row", both compute
// consecutive_failures=1, and the second INSERT ... ON CONFLICT DO UPDATE
// would silently overwrite the first — losing one of the two failures.
// Real Postgres row locking forces the interleaving this needs: the first
// caller's transaction creates and locks the row, the second caller's
// call blocks on that lock, and only proceeds once the first commits — at
// which point it must see consecutive_failures=1 already recorded and
// increment to 2, not overwrite it back to 1. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresRecordGateOutcome_ConcurrentFirstFailuresBothCount(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
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

	// secondConnDB is a second, genuinely separate physical connection to
	// the SAME schema (search_path is per-connection session state, so a
	// pool-wide MaxOpenConns bump on one *sqlx.DB would not do this
	// safely — see cancel_postgres_test.go for the same pattern).
	var schema string
	if err := db.Get(&schema, "SELECT current_schema()"); err != nil {
		t.Fatalf("read isolated schema name: %v", err)
	}
	secondConnDB, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	secondConnDB.SetMaxOpenConns(1)
	secondConnDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = secondConnDB.Close() })
	if _, err := secondConnDB.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("set second connection search_path: %v", err)
	}
	secondRepo := runssqlite.NewWithDB(secondConnDB, secondConnDB)

	const workspaceID = "ws-gate-race"
	const gate = "agent_ceiling"

	tx1, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin first tx: %v", err)
	}
	defer func() { _ = tx1.Rollback() }()
	if err := repo.RecordGateOutcomeTx(ctx, tx1, workspaceID, gate, false); err != nil {
		t.Fatalf("first outcome (holds the row lock, uncommitted): %v", err)
	}

	var (
		secondDone = make(chan struct{})
		secondErr  error
	)
	go func() {
		defer close(secondDone)
		secondErr = secondRepo.RecordGateOutcome(ctx, workspaceID, gate, false)
	}()

	// Give the second call time to reach Postgres and block on the row
	// lock the first transaction holds, before that transaction commits.
	// Without this window the two calls could run in either order and the
	// test would not exercise the lock-wait path this bug depends on.
	select {
	case <-secondDone:
		t.Fatal("second RecordGateOutcome returned before the first committed; it never blocked on the row lock")
	case <-time.After(200 * time.Millisecond):
	}

	if err := tx1.Commit(); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	<-secondDone
	if secondErr != nil {
		t.Fatalf("second outcome: %v", secondErr)
	}

	state, err := repo.GetGateFailureState(ctx, workspaceID, gate)
	if err != nil {
		t.Fatalf("get gate failure state: %v", err)
	}
	if state.ConsecutiveFailures != 2 {
		t.Fatalf("consecutive_failures = %d, want 2 (both concurrent first-failures must count)", state.ConsecutiveFailures)
	}
}
