package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresContinuationScopeMigration is the PostgreSQL twin of
// TestContinuationScopeMigrationBackfillsLegacyRunsAndReplays. It verifies
// that an upgraded database repairs queued and claimed runs, then replays the
// migration cleanly on the next boot. It skips unless a test DSN is set.
func TestPostgresContinuationScopeMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init current office schema: %v", err)
	}

	if _, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_run_failure_scope_status`); err != nil {
		t.Fatalf("drop continuation scope index from legacy schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE runs DROP COLUMN continuation_scope`); err != nil {
		t.Fatalf("drop continuation_scope from legacy schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO runs (
			id, agent_profile_id, reason, payload, status, context_snapshot, requested_at
		) VALUES
			('legacy-routine', 'agent-routine', 'routine_trigger', '{}', 'queued',
			 '{"routine_id":"routine-1"}', CURRENT_TIMESTAMP),
			('legacy-agent', 'agent-plain', 'self', '{}', 'claimed',
			 '{}', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed legacy runs: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	assertContinuationScopePostgres(t, db, "legacy-routine", "routine:routine-1")
	assertContinuationScopePostgres(t, db, "legacy-agent", "agent:agent-plain")

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	assertContinuationScopePostgres(t, db, "legacy-routine", "routine:routine-1")
	assertContinuationScopePostgres(t, db, "legacy-agent", "agent:agent-plain")
}

func assertContinuationScopePostgres(t *testing.T, db *sqlx.DB, runID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(context.Background(),
		`SELECT continuation_scope FROM runs WHERE id = $1`, runID).Scan(&got); err != nil {
		t.Fatalf("read continuation scope for %q: %v", runID, err)
	}
	if got != want {
		t.Errorf("continuation_scope for %q = %q, want %q", runID, got, want)
	}
}
