package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// The cached-token rollup (IncrementTaskSessionUsageTx) is dialect-sensitive in
// a way the SQLite tests in session_usage_test.go and session_test.go cannot
// exercise: SQLite's INTEGER is 64-bit, so a column declared INTEGER never
// overflows there. On Postgres, INTEGER is int4 (max 2,147,483,647), and
// office_cost_events.tokens_cached_in routinely accumulates well past that
// for a single long-running session (the reported bug measured up to
// 98,805,109 on one already-completed task). Without this file the type of
// the new column was never exercised against Postgres at all.
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

// TestPostgresIncrementTaskSessionUsage_AccumulatesValuesBeyondInt32Range
// covers F8's other failure mode: IncrementTaskSessionUsageTx sets tokens_in,
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
	if err := repo.IncrementTaskSessionUsageTx(ctx, nil, "sess-inc-overflow-pg", 100, beyondInt32, 200, 50); err != nil {
		t.Fatalf("IncrementTaskSessionUsageTx must not error on a cached-token delta beyond int32 range: %v", err)
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
