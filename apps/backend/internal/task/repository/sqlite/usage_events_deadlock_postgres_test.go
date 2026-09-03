package sqlite

// TestPostgresCreateTaskUsageEvent_RealDeadlockIsClassifiedTransientAndRetried
// closes Review Round 2's M2 finding: the other AC-32 transient-retry tests
// (usage_events_transient_test.go) only prove the retry loop's control flow
// against an injected failpoint error, never against a genuine
// driver-reported 40001/40P01. internal/db's classifier already has focused
// unit coverage for those codes via synthetic *pgconn.PgError values
// (migration_errors_test.go); what was missing was proof that a real
// Postgres deadlock, raised against the exact unmodified production write
// path, actually reaches that classifier and gets retried rather than
// surfaced as a fatal error.
//
// Constructing a genuine deadlock here needs two real, separate connections
// (SQLite's single test connection can't produce true concurrent contention)
// and a lock-escalation pattern that survives Postgres's row-lock
// compatibility rules:
//
//   - CreateTaskUsageEvent's ledger INSERT references task_sessions via a
//     foreign key, which takes FOR KEY SHARE on the referenced session row.
//   - Its rollup UPDATE (IncrementTaskSessionUsageTx) never touches the
//     session's primary key, so it only ever requests FOR NO KEY UPDATE -
//     the weakest lock that still conflicts with a concurrent FOR NO KEY
//     UPDATE (row-lock modes, strongest to weakest: FOR UPDATE > FOR NO KEY
//     UPDATE > FOR SHARE > FOR KEY SHARE; FOR NO KEY UPDATE conflicts with
//     itself but not with FOR KEY SHARE).
//   - A plain antagonist UPDATE that also avoids the primary key only ever
//     takes FOR NO KEY UPDATE too, which is compatible with the production
//     transaction's held FOR KEY SHARE - it will not deadlock with N
//     concurrent real writers alone. The antagonist here deliberately
//     escalates to FOR UPDATE via a second, explicit `SELECT ... FOR
//     UPDATE`, which does conflict with production's held FOR KEY SHARE,
//     closing the cycle.
//
// The hook is paused only on the antagonist's side of the transaction
// timeline: production's ledger insert runs first and pauses inside
// insertUsageEventAndRollup (before the rollup UPDATE) so the antagonist's
// first statement can be issued and its lock granted; production is then
// released into its rollup UPDATE (which blocks on the antagonist's FOR NO
// KEY UPDATE) before the antagonist's FOR UPDATE is issued (which then
// blocks on production's held FOR KEY SHARE) - so production's wait
// registers first and Postgres's deadlock detector reliably picks it as the
// victim, exercising exactly the retry path AC-32(c) describes. Verified via
// the real Postgres server log during development: a genuine `deadlock
// detected` (40P01) naming production's own rollup UPDATE statement as one
// of the two waiters.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresCreateTaskUsageEvent_RealDeadlockIsClassifiedTransientAndRetried(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-deadlock-pg", "session-deadlock-pg")

	// The antagonist needs its own physical connection: repo.db (used by
	// testutil.OpenIsolatedPostgres) caps MaxOpenConns at 1, so sharing it
	// would starve production of a connection entirely instead of
	// constructing genuine cross-connection Postgres lock contention.
	var searchPath string
	if err := repo.db.Get(&searchPath, `SHOW search_path`); err != nil {
		t.Fatalf("show search_path: %v", err)
	}
	antagonistDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open antagonist connection: %v", err)
	}
	defer func() { _ = antagonistDB.Close() }()
	if _, err := antagonistDB.Exec("SET search_path TO " + searchPath); err != nil {
		t.Fatalf("set antagonist search_path: %v", err)
	}

	antagonistTx, err := antagonistDB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin antagonist transaction: %v", err)
	}
	t.Cleanup(func() { _ = antagonistTx.Rollback() })
	if _, err := antagonistTx.Exec(
		`UPDATE task_sessions SET tokens_in = tokens_in WHERE id = $1`, "session-deadlock-pg",
	); err != nil {
		t.Fatalf("antagonist FOR NO KEY UPDATE statement: %v", err)
	}

	insertedCh := make(chan struct{})
	releaseCh := make(chan struct{})
	attempts := 0
	var hookPaused bool
	repo.usageEventPreRollupHook = func() {
		attempts++
		// Only pause on the first attempt: if production loses the
		// deadlock and retries, insertUsageEventAndRollup calls this hook
		// again on every subsequent attempt, and closing insertedCh a
		// second time would panic.
		if hookPaused {
			return
		}
		hookPaused = true
		close(insertedCh)
		<-releaseCh
	}

	mainErrCh := make(chan error, 1)
	go func() {
		event := newTestUsageEvent("evt-deadlock-pg", "task-deadlock-pg", "session-deadlock-pg")
		mainErrCh <- repo.CreateTaskUsageEvent(context.Background(), event)
	}()

	<-insertedCh
	close(releaseCh) // production proceeds into its rollup UPDATE, blocking on the antagonist's held FOR NO KEY UPDATE

	time.Sleep(200 * time.Millisecond) // ensure production's wait registers before the antagonist's escalation

	antagonistErrCh := make(chan error, 1)
	go func() {
		_, escalateErr := antagonistTx.Exec(`SELECT id FROM task_sessions WHERE id = $1 FOR UPDATE`, "session-deadlock-pg")
		antagonistErrCh <- escalateErr
	}()

	var mainErr, antagonistErr error
	var mainDone, antagonistDone bool
	for !mainDone || !antagonistDone {
		select {
		case mainErr = <-mainErrCh:
			mainDone = true
		case antagonistErr = <-antagonistErrCh:
			antagonistDone = true
			// Release the antagonist's transaction the instant its own call
			// returns rather than after production's: if the deadlock
			// detector picks production as the victim, production's retry
			// needs this FOR UPDATE lock released to proceed, so waiting on
			// mainErrCh first would self-deadlock the test harness (not
			// Postgres).
			if antagonistErr != nil {
				_ = antagonistTx.Rollback()
			} else {
				_ = antagonistTx.Commit()
			}
		}
	}

	if mainErr != nil {
		t.Fatalf("CreateTaskUsageEvent must transparently retry past a real deadlock, got: %v", mainErr)
	}
	if attempts < 2 {
		t.Fatalf("insertUsageEventAndRollup ran %d attempt(s), want at least 2 - "+
			"either no real deadlock occurred or the transient retry did not fire", attempts)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events`); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 (the aborted first attempt must not leave a row)", count)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-deadlock-pg")
	if tokensIn != 100 || tokensCachedIn != 25 || tokensOut != 30 || costSubcents != 42 {
		t.Errorf("rollup = (%d,%d,%d,%d), want (100,25,30,42) - "+
			"the aborted first attempt must not have partially or doubly applied", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}
