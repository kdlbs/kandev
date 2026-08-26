package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestContinuationScopeMigrationBackfillsLegacyRunsAndReplays(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("create current schema: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE runs DROP COLUMN continuation_scope`); err != nil {
		t.Fatalf("drop continuation_scope from legacy schema: %v", err)
	}
	if _, err := db.Exec(`
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
	assertContinuationScope(t, db, "legacy-routine", "routine:routine-1")
	assertContinuationScope(t, db, "legacy-agent", "agent:agent-plain")

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	assertContinuationScope(t, db, "legacy-routine", "routine:routine-1")
	assertContinuationScope(t, db, "legacy-agent", "agent:agent-plain")
}

func assertContinuationScope(t *testing.T, db *sqlx.DB, runID, want string) {
	t.Helper()
	var got string
	if err := db.Get(&got, `SELECT continuation_scope FROM runs WHERE id = ?`, runID); err != nil {
		t.Fatalf("read continuation scope for %q: %v", runID, err)
	}
	if got != want {
		t.Errorf("continuation_scope for %q = %q, want %q", runID, got, want)
	}
}
