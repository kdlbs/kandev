package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresReapStaleCheckouts is the PostgreSQL twin of
// TestReapStaleCheckouts_ReapsWithNoInFlightRun and
// TestReapStaleCheckouts_SkipsTaskWithInFlightRun (checkout_reaper_test.go).
// ReapStaleCheckouts resolved a run's task id with a literal
// `json_extract(w.payload, '$.task_id')`, so on Postgres every reaper tick
// failed with "function json_extract(text, unknown) does not exist"
// (SQLSTATE 42883) and the backstop never ran. Postgres rejects the whole
// statement at parse time, so this fails on the first tick rather than
// returning wrong rows. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresReapStaleCheckouts(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo := newPostgresSearchTestRepo(t, db)
	ctx := context.Background()

	const (
		staleID    = "pg-reap-stale"
		liveID     = "pg-reap-live"
		staleAgent = "agent-pg-gone"
		liveAgent  = "agent-pg-active"
	)
	// Postgres TIMESTAMP has no datetime('now', '-1 hour'); bind the same
	// one-hour-old instant the SQLite fixtures express textually.
	checkoutAt := time.Now().UTC().Add(-time.Hour)

	insertPostgresTask(t, repo, ctx, staleID, "pg-ws-reap", "Stale checkout")
	insertPostgresTask(t, repo, ctx, liveID, "pg-ws-reap", "Stale checkout, live run")

	// The reaped row's checkout_run_id points at a run that no longer exists,
	// so the run-id arm of the in-flight check cannot suppress the reap.
	acquired, err := repo.CheckoutTaskForRun(ctx, staleID, staleAgent, "run-pg-finished")
	if err != nil {
		t.Fatalf("checkout stale task: %v", err)
	}
	if !acquired {
		t.Fatal("expected stale task checkout to succeed")
	}
	execPostgres(t, ctx, repo, `UPDATE tasks SET checkout_at = ? WHERE id = ?`, checkoutAt, staleID)

	acquired, err = repo.CheckoutTaskForRun(ctx, liveID, liveAgent, "")
	if err != nil {
		t.Fatalf("checkout live task: %v", err)
	}
	if !acquired {
		t.Fatal("expected live task checkout to succeed")
	}
	execPostgres(t, ctx, repo, `UPDATE tasks SET checkout_at = ? WHERE id = ?`, checkoutAt, liveID)
	execPostgres(t, ctx, repo, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES ('run-pg-live', ?, 'task_assigned', ?, 'claimed', ?)
	`, liveAgent, `{"task_id":"`+liveID+`"}`, checkoutAt)

	count, err := repo.ReapStaleCheckouts(ctx, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("ReapStaleCheckouts: %v", err)
	}
	if count != 1 {
		t.Fatalf("reaped count = %d, want 1 (only the checkout with no in-flight run)", count)
	}

	var reapedAgent, reapedAt, reapedRun sql.NullString
	if err := db.QueryRowxContext(ctx, db.Rebind(`
		SELECT checkout_agent_id, checkout_at, checkout_run_id FROM tasks WHERE id = ?
	`), staleID).Scan(&reapedAgent, &reapedAt, &reapedRun); err != nil {
		t.Fatalf("read reaped task: %v", err)
	}
	if reapedAgent.Valid {
		t.Errorf("%s.checkout_agent_id = %q, want NULL", staleID, reapedAgent.String)
	}
	if reapedAt.Valid {
		t.Errorf("%s.checkout_at = %q, want NULL", staleID, reapedAt.String)
	}
	if reapedRun.Valid {
		t.Errorf("%s.checkout_run_id = %q, want NULL", staleID, reapedRun.String)
	}

	var heldAgent string
	var heldAt sql.NullString
	if err := db.QueryRowxContext(ctx, db.Rebind(`
		SELECT checkout_agent_id, checkout_at FROM tasks WHERE id = ?
	`), liveID).Scan(&heldAgent, &heldAt); err != nil {
		t.Fatalf("read retained task: %v", err)
	}
	if heldAgent != liveAgent {
		t.Errorf("%s.checkout_agent_id = %q, want %q (its holder still has a claimed run)", liveID, heldAgent, liveAgent)
	}
	if !heldAt.Valid {
		t.Errorf("%s.checkout_at = NULL, want the retained checkout timestamp", liveID)
	}
}
