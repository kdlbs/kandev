package sqlite

// AC-32 coverage against a real Postgres instance: the detector's typed
// pgconn.PgError branch (ConstraintName == uniq_task_usage_events_usage_event_id)
// is only reachable through an actual PostgreSQL driver error, never through
// SQLite's string-matched fallback.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func insertPostgresUsageEventRow(t *testing.T, repo *Repository, eventID, taskID string) error {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), eventID, taskID, nil, nil, now, now)
	return err
}

func TestPostgresIsUsageEventUniqueViolation_DuplicateUsageEventID(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresTaskSession(t, repo, "task-dup-unique-pg", "session-dup-unique-pg")

	if err := insertPostgresUsageEventRow(t, repo, "evt-dup-pg", "task-dup-unique-pg"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err = insertPostgresUsageEventRow(t, repo, "evt-dup-pg", "task-dup-unique-pg")
	if err == nil {
		t.Fatal("expected a unique-constraint error inserting a duplicate usage_event_id, got nil")
	}
	if !isUsageEventUniqueViolation(err) {
		t.Errorf("isUsageEventUniqueViolation(%v) = false, want true", err)
	}
}

func TestPostgresIsUsageEventUniqueViolation_ForeignKeyNotMisclassified(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	err = insertPostgresUsageEventRow(t, repo, "evt-no-task-pg", "task-does-not-exist-pg")
	if err == nil {
		t.Fatal("expected a foreign-key error inserting a row against an unknown task_id, got nil")
	}
	if isUsageEventUniqueViolation(err) {
		t.Errorf("isUsageEventUniqueViolation(%v) = true, want false (this is a foreign-key violation)", err)
	}
}
