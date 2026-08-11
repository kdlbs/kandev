package sqlite

import (
	"context"
	"testing"
	"time"
)

// TestIncrementTaskSessionUsage_AccumulatesCachedInAcrossCalls confirms
// tokens_cached_in compounds the same way tokens_in/tokens_out/cost_subcents
// already do. Before the fix this column didn't exist and the parameter
// couldn't be threaded at all.
func TestIncrementTaskSessionUsage_AccumulatesCachedInAcrossCalls(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-cached-usage", "sess-cached-usage", "turn-cached-usage")

	if err := repo.IncrementTaskSessionUsage(ctx, "sess-cached-usage", 100, 50_000_000, 200, 50); err != nil {
		t.Fatalf("first increment: %v", err)
	}
	if err := repo.IncrementTaskSessionUsage(ctx, "sess-cached-usage", 10, 2_000, 20, 5); err != nil {
		t.Fatalf("second increment: %v", err)
	}

	var tokensIn, tokensCachedIn, tokensOut, costSubcents int64
	err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_in, tokens_cached_in, tokens_out, cost_subcents FROM task_sessions WHERE id = ?`),
		"sess-cached-usage").Scan(&tokensIn, &tokensCachedIn, &tokensOut, &costSubcents)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if tokensIn != 110 || tokensCachedIn != 50_002_000 || tokensOut != 220 || costSubcents != 55 {
		t.Errorf("totals = (%d,%d,%d,%d), want (110,50002000,220,55)",
			tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}

// createOfficeCostEventsTable creates a minimal standalone copy of
// office_cost_events (owned by internal/office/repository/sqlite) so this
// package's migration tests can exercise the guarded backfill without
// importing the office package (which would create an import cycle back
// into internal/task). Column set mirrors office/repository/sqlite/base.go
// createCostTables.
func createOfficeCostEventsTable(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`
		CREATE TABLE IF NOT EXISTS office_cost_events (
			id TEXT PRIMARY KEY,
			session_id TEXT DEFAULT '',
			task_id TEXT DEFAULT '',
			agent_profile_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			model TEXT DEFAULT '',
			provider TEXT DEFAULT '',
			tokens_in INTEGER DEFAULT 0,
			tokens_cached_in INTEGER DEFAULT 0,
			tokens_out INTEGER DEFAULT 0,
			cost_subcents INTEGER NOT NULL DEFAULT 0,
			estimated INTEGER NOT NULL DEFAULT 0,
			occurred_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`); err != nil {
		t.Fatalf("create office_cost_events: %v", err)
	}
}

func insertOfficeCostEvent(t *testing.T, repo *Repository, id, sessionID string, tokensCachedIn int64) {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO office_cost_events (id, session_id, tokens_cached_in, occurred_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`), id, sessionID, tokensCachedIn, now, now)
	if err != nil {
		t.Fatalf("insert office_cost_event %s: %v", id, err)
	}
}

// TestBackfillSessionTokensCachedIn_CreatesSessionIDIndex guards the
// backfill's boot-time cost: without an index on
// office_cost_events(session_id), the correlated subquery in the backfill's
// UPDATE falls back to a full table scan per task_sessions row (a measured
// ~450x slowdown at 4,000 sessions / 80,000 events - 76.72s vs 0.17s). The
// backfill runs unconditionally on every boot (see runMigrations), so a
// missing index is a startup-latency regression, not just a slow query.
func TestBackfillSessionTokensCachedIn_CreatesSessionIDIndex(t *testing.T) {
	repo := newRepoForSessionTests(t)
	createOfficeCostEventsTable(t, repo)

	if err := repo.backfillSessionTokensCachedIn(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var name string
	err := repo.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'office_cost_events' AND sql LIKE '%session_id%'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("expected an index on office_cost_events(session_id) to exist after the backfill runs, got: %v", err)
	}
}

// TestBackfillSessionTokensCachedIn_SumsLedger proves the backfill invariant
// directly: task_sessions.tokens_cached_in must equal the sum of
// office_cost_events.tokens_cached_in for that session. This is the
// reconciliation the card asked for, exercised at the SQL layer.
func TestBackfillSessionTokensCachedIn_SumsLedger(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-backfill", "sess-backfill", "turn-backfill")
	createOfficeCostEventsTable(t, repo)
	insertOfficeCostEvent(t, repo, "cost-1", "sess-backfill", 98_805_109)
	insertOfficeCostEvent(t, repo, "cost-2", "sess-backfill", 1_194_891)
	// A row for a different session must not leak into this session's total.
	insertOfficeCostEvent(t, repo, "cost-other", "sess-other", 555)

	if err := repo.backfillSessionTokensCachedIn(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-backfill",
	).Scan(&got); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if got != 100_000_000 {
		t.Errorf("tokens_cached_in = %d, want 100000000 (sum of office_cost_events for this session)", got)
	}

	// Idempotent: re-running against an unchanged ledger must not
	// double-count the already-correct total.
	if err := repo.backfillSessionTokensCachedIn(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-backfill",
	).Scan(&got); err != nil {
		t.Fatalf("read row after second pass: %v", err)
	}
	if got != 100_000_000 {
		t.Errorf("tokens_cached_in after second backfill pass = %d, want unchanged 100000000", got)
	}
}

// TestBackfillSessionTokensCachedIn_CorrectsDriftedRollup covers the case an
// unconditional recompute exists to repair: a session whose rollup went out
// of sync with the ledger without ever erroring. IncrementTaskSessionUsage
// is a best-effort `UPDATE ... WHERE id = ?` that silently matches zero rows
// if the session row doesn't exist yet (see
// TestIncrementTaskSessionUsage_UnknownSessionNoError), and the office
// subscriber only logs a Warn when that write fails while the ledger insert
// has already committed (event_subscribers.go handlePromptUsage). A backfill
// gated on "already nonzero" would treat this drifted-but-nonzero row as
// already correct and skip it forever; the unconditional recompute must
// instead overwrite it with the true ledger sum.
func TestBackfillSessionTokensCachedIn_CorrectsDriftedRollup(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-drifted", "sess-drifted", "turn-drifted")
	createOfficeCostEventsTable(t, repo)
	insertOfficeCostEvent(t, repo, "cost-1", "sess-drifted", 98_805_109)
	insertOfficeCostEvent(t, repo, "cost-2", "sess-drifted", 1_194_891)

	// Simulate the drift directly: the rollup already holds a nonzero but
	// wrong value, as it would after a missed increment.
	if err := repo.IncrementTaskSessionUsage(ctx, "sess-drifted", 0, 2_000, 0, 0); err != nil {
		t.Fatalf("seed drifted rollup: %v", err)
	}

	var before int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-drifted",
	).Scan(&before); err != nil {
		t.Fatalf("read row before backfill: %v", err)
	}
	if before != 2_000 {
		t.Fatalf("test setup bug: rollup = %d, want the drifted seed value 2000", before)
	}

	if err := repo.backfillSessionTokensCachedIn(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-drifted",
	).Scan(&got); err != nil {
		t.Fatalf("read row after backfill: %v", err)
	}
	if got != 100_000_000 {
		t.Errorf("tokens_cached_in after backfill = %d, want 100000000 (the drifted 2000 must be "+
			"overwritten with the full ledger sum, not skipped as already-nonzero)", got)
	}
}

// TestBackfillSessionTokensCachedIn_NoOpWithoutLedgerTable reproduces the
// fresh-boot ordering case: the task repository's migrations run before the
// office repository has created office_cost_events (see
// internal/backendapp/storage.go). The guarded backfill must not error and
// must not touch task_sessions.
func TestBackfillSessionTokensCachedIn_NoOpWithoutLedgerTable(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-no-ledger", "sess-no-ledger", "turn-no-ledger")

	// office_cost_events was never created in this repo — simulates a fresh
	// boot where the task repo's migrations run first.
	if err := repo.backfillSessionTokensCachedIn(); err != nil {
		t.Fatalf("backfill without ledger table should not error: %v", err)
	}

	var got int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-no-ledger",
	).Scan(&got); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if got != 0 {
		t.Errorf("tokens_cached_in = %d, want 0 (no ledger to backfill from)", got)
	}
}

// TestBackfillSessionTokensCachedIn_ZeroesRollupWithNoMatchingLedgerRows pins
// the behaviour when the ledger table exists but has no rows for a session
// whose rollup is already nonzero (e.g. the ledger rows for that session were
// deleted separately from the session itself - see workspace_deletion.go,
// which deletes office_cost_events before the task-side workspace in two
// separate transactions). The unconditional recompute is a COALESCE(...,0),
// so it deliberately zeroes tokens_cached_in in this case rather than leaving
// the stale value: the rollup is supposed to always equal the ledger sum for
// that session, and an empty ledger sums to zero.
func TestBackfillSessionTokensCachedIn_ZeroesRollupWithNoMatchingLedgerRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-orphaned", "sess-orphaned", "turn-orphaned")
	createOfficeCostEventsTable(t, repo)

	// Rollup already holds a nonzero value, but no office_cost_events rows
	// exist for this session (simulates the ledger having been deleted).
	if err := repo.IncrementTaskSessionUsage(ctx, "sess-orphaned", 0, 12_345, 0, 0); err != nil {
		t.Fatalf("seed rollup: %v", err)
	}

	if err := repo.backfillSessionTokensCachedIn(); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got int64
	if err := repo.ro.QueryRowx(repo.ro.Rebind(
		`SELECT tokens_cached_in FROM task_sessions WHERE id = ?`), "sess-orphaned",
	).Scan(&got); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if got != 0 {
		t.Errorf("tokens_cached_in = %d, want 0 (no matching ledger rows sums to zero)", got)
	}
}

// TestOfficeCostEventsTableExists confirms the guard used by the backfill.
func TestOfficeCostEventsTableExists(t *testing.T) {
	repo := newRepoForSessionTests(t)

	exists, err := repo.officeCostEventsTableExists()
	if err != nil {
		t.Fatalf("officeCostEventsTableExists: %v", err)
	}
	if exists {
		t.Fatal("expected office_cost_events to not exist before the office repo creates it")
	}

	createOfficeCostEventsTable(t, repo)

	exists, err = repo.officeCostEventsTableExists()
	if err != nil {
		t.Fatalf("officeCostEventsTableExists after create: %v", err)
	}
	if !exists {
		t.Fatal("expected office_cost_events to exist after creation")
	}
}
