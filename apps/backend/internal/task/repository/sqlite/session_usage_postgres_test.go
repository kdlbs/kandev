package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// The cached-token rollup (BackfillSessionTokensCachedIn,
// IncrementTaskSessionUsage) is dialect-sensitive in a way the SQLite tests in
// session_usage_test.go and session_test.go cannot exercise: SQLite's INTEGER
// is 64-bit, so a column declared INTEGER never overflows there. On Postgres,
// INTEGER is int4 (max 2,147,483,647), and office_cost_events.tokens_cached_in
// routinely accumulates well past that for a single long-running session (the
// reported bug measured up to 98,805,109 on one already-completed task).
// Without this file the type of the new column was never exercised against
// Postgres at all.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

func seedPostgresTaskSession(t *testing.T, repo *Repository, taskID, sessionID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, '', 'test task', ?, ?)
	`), taskID, now, now); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), sessionID, taskID, now, now); err != nil {
		t.Fatalf("seed session %s: %v", sessionID, err)
	}
}

// TestPostgresBackfillSessionTokensCachedIn_SumsLedger is the Postgres
// counterpart to TestBackfillSessionTokensCachedIn_SumsLedger: proves the
// correlated-subquery UPDATE and its CREATE INDEX IF NOT EXISTS both work
// against a real Postgres instance, not just SQLite.
func TestPostgresBackfillSessionTokensCachedIn_SumsLedger(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-backfill-pg", "sess-backfill-pg")
	createOfficeCostEventsTable(t, repo)
	insertOfficeCostEvent(t, repo, "cost-1", "sess-backfill-pg", 98_805_109)
	insertOfficeCostEvent(t, repo, "cost-2", "sess-backfill-pg", 1_194_891)
	insertOfficeCostEvent(t, repo, "cost-other", "sess-other-pg", 555)

	if _, err := repo.BackfillSessionTokensCachedIn(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-backfill-pg",
	).Scan(&got); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if got != 100_000_000 {
		t.Errorf("tokens_cached_in = %d, want 100000000 (sum of office_cost_events for this session)", got)
	}
}

// TestPostgresBackfillSessionTokensCachedIn_HandlesValuesBeyondInt32Range is
// the regression test for F8: task_sessions.tokens_cached_in was declared
// INTEGER (int4 on Postgres, max 2,147,483,647). Because the backfill is a
// single UPDATE across the whole task_sessions table, one session whose
// ledger sum exceeds the int4 ceiling made the entire statement fail with
// "integer out of range" — swallowed to a Warn by MigrateLogger.Apply, so the
// self-healing recompute silently stopped working for every session, not just
// the overflowing one. Fails on an INTEGER column, passes once the column is
// BIGINT.
func TestPostgresBackfillSessionTokensCachedIn_HandlesValuesBeyondInt32Range(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-overflow-pg", "sess-overflow-pg")
	seedPostgresTaskSession(t, repo, "task-normal-pg", "sess-normal-pg")
	createOfficeCostEventsTable(t, repo)

	// 200 events of 14,000,000 cached tokens each = 2,800,000,000, well past
	// the int32 ceiling of 2,147,483,647 - the shape the card's own live-instance
	// measurement (98,805,109 on a single completed task) extrapolates to over
	// a long-running session.
	const perEvent = int64(14_000_000)
	const eventCount = 200
	const wantOverflow = perEvent * eventCount
	for i := 0; i < eventCount; i++ {
		insertOfficeCostEvent(t, repo, fmt.Sprintf("cost-overflow-%d", i), "sess-overflow-pg", perEvent)
	}
	// A normal-sized session must still backfill correctly in the same pass -
	// the whole point of F8 is that one overflowing row previously took the
	// entire UPDATE down with it.
	insertOfficeCostEvent(t, repo, "cost-normal", "sess-normal-pg", 1_000)

	if _, err := repo.BackfillSessionTokensCachedIn(context.Background()); err != nil {
		t.Fatalf("backfill must not error on a ledger sum beyond int32 range: %v", err)
	}

	var gotOverflow, gotNormal int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-overflow-pg",
	).Scan(&gotOverflow); err != nil {
		t.Fatalf("read overflow row: %v", err)
	}
	if gotOverflow != wantOverflow {
		t.Errorf("tokens_cached_in = %d, want %d (sum beyond int32 range must survive the backfill)",
			gotOverflow, wantOverflow)
	}
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-normal-pg",
	).Scan(&gotNormal); err != nil {
		t.Fatalf("read normal row: %v", err)
	}
	if gotNormal != 1_000 {
		t.Errorf("tokens_cached_in for the non-overflowing session = %d, want 1000 "+
			"(a sibling session's overflow must not abort the whole backfill statement)", gotNormal)
	}
}

// TestPostgresIncrementTaskSessionUsage_AccumulatesValuesBeyondInt32Range
// covers F8's other failure mode: IncrementTaskSessionUsage sets tokens_in,
// tokens_cached_in, tokens_out and cost_subcents in one UPDATE. On an INTEGER
// column that UPDATE fails once tokens_cached_in crosses the int32 ceiling,
// silently stopping tokens_in/tokens_out/cost_subcents from updating too -
// columns the card states reconcile correctly today. Fails on an INTEGER
// column, passes once the column is BIGINT.
func TestPostgresIncrementTaskSessionUsage_AccumulatesValuesBeyondInt32Range(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTaskSession(t, repo, "task-inc-overflow-pg", "sess-inc-overflow-pg")

	const beyondInt32 = int64(2_800_000_000)
	if err := repo.IncrementTaskSessionUsage(ctx, "sess-inc-overflow-pg", 100, beyondInt32, 200, 50); err != nil {
		t.Fatalf("IncrementTaskSessionUsage must not error on a cached-token delta beyond int32 range: %v", err)
	}

	var tokensIn, tokensCachedIn, tokensOut, costSubcents int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_in, tokens_cached_in, tokens_out, cost_subcents FROM task_sessions WHERE id = ?`),
		"sess-inc-overflow-pg").Scan(&tokensIn, &tokensCachedIn, &tokensOut, &costSubcents); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if tokensIn != 100 || tokensCachedIn != beyondInt32 || tokensOut != 200 || costSubcents != 50 {
		t.Errorf("totals = (%d,%d,%d,%d), want (100,%d,200,50) - a cached-token delta beyond int32 "+
			"range must not roll back tokens_in/tokens_out/cost_subcents in the same statement",
			tokensIn, tokensCachedIn, tokensOut, costSubcents, beyondInt32)
	}
}
