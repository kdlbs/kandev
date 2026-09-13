package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestFreshSchema_RunsHasWakeWaveColumnsAndIndex(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("create fresh schema: %v", err)
	}

	assertWakeWaveDefaults(t, db, "fresh-run")
	assertWakeWaveUniqueIndex(t, db)
}

func TestWakeWaveColumnsMigration_ConvergesLegacySchemaAndReplays(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("create current schema: %v", err)
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_run_wake_wave`); err != nil {
		t.Fatalf("drop wake wave index from legacy schema: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE runs DROP COLUMN wake_wave_key`); err != nil {
		t.Fatalf("drop wake_wave_key from legacy schema: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE runs DROP COLUMN wake_wave_string`); err != nil {
		t.Fatalf("drop wake_wave_string from legacy schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES ('legacy-run', 'agent-1', 'task_assigned', '{}', 'queued', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed legacy run: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	assertWakeWaveDefaults(t, db, "legacy-run")
	assertWakeWaveUniqueIndex(t, db)

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	assertWakeWaveDefaults(t, db, "legacy-run")
	// The index created by the first migration run must still be in force
	// after replaying migrateWakeWaveColumns a second time (CREATE UNIQUE
	// INDEX IF NOT EXISTS is idempotent) — re-inserting the same
	// (wake_wave_key, agent_profile_id) pair assertWakeWaveUniqueIndex
	// already proved unique must still be rejected.
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at, wake_wave_key, wake_wave_string)
		VALUES ('wave-a-replay', 'agent-x', 'task_children_completed', '{}', 'queued', CURRENT_TIMESTAMP, 'wave-key-1', 'p|1')
	`); err == nil {
		t.Fatal("expected unique-index violation to survive migration replay, got none")
	}
}

func assertWakeWaveDefaults(t *testing.T, db *sqlx.DB, runID string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES (?, 'agent-defaults', 'task_assigned', '{}', 'queued', CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO NOTHING
	`, runID); err != nil {
		t.Fatalf("ensure run %q exists: %v", runID, err)
	}
	var key, str string
	if err := db.QueryRow(
		`SELECT wake_wave_key, wake_wave_string FROM runs WHERE id = ?`, runID,
	).Scan(&key, &str); err != nil {
		t.Fatalf("read wake wave columns for %q: %v", runID, err)
	}
	if key != "" || str != "" {
		t.Errorf("run %q wake wave columns = (%q, %q), want empty defaults", runID, key, str)
	}
}

func assertWakeWaveUniqueIndex(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at, wake_wave_key, wake_wave_string)
		VALUES ('wave-a', 'agent-x', 'task_children_completed', '{}', 'queued', CURRENT_TIMESTAMP, 'wave-key-1', 'p|1')
	`); err != nil {
		t.Fatalf("insert first wave run: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at, wake_wave_key, wake_wave_string)
		VALUES ('wave-b', 'agent-x', 'task_children_completed', '{}', 'queued', CURRENT_TIMESTAMP, 'wave-key-1', 'p|1')
	`); err == nil {
		t.Fatal("expected unique-index violation for duplicate (wake_wave_key, agent_profile_id), got none")
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at, wake_wave_key, wake_wave_string)
		VALUES ('wave-c', 'agent-y', 'task_children_completed', '{}', 'queued', CURRENT_TIMESTAMP, 'wave-key-1', 'p|1')
	`); err != nil {
		t.Fatalf("insert wave run for a different agent should succeed: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES ('empty-wave-1', 'agent-x', 'task_assigned', '{}', 'queued', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("insert first empty-wave-key run: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES ('empty-wave-2', 'agent-x', 'task_assigned', '{}', 'queued', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("second empty-wave-key run for same agent should succeed (partial index excludes empty key): %v", err)
	}
}
