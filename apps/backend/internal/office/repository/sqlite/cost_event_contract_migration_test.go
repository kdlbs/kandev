package sqlite_test

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestCostEventContractMigration_LegacyRowsGetNullColumns covers the
// legacy-column contract from docs/specs/office/requirements/costs.md: an office_cost_events
// table that predates the cache-split / cost-provenance / turn-attribution
// columns gets them added as nullable, a pre-existing row is left with NULL
// (never 0) in every new column, the partial unique index on
// usage_event_id exists, and the migration replays cleanly on a second boot.
func TestCostEventContractMigration_LegacyRowsGetNullColumns(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// Seed the pre-migration shape of office_cost_events (as it exists on
	// main today, per base.go's createCostTables) and a legacy row, before
	// the repo (and its migration) ever runs.
	if _, err := db.Exec(`
		CREATE TABLE office_cost_events (
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
		t.Fatalf("seed legacy office_cost_events: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO office_cost_events (
			id, task_id, tokens_in, tokens_cached_in, tokens_out,
			cost_subcents, estimated, occurred_at, created_at
		) VALUES (
			'legacy-1', 'task-legacy', 100, 40, 50,
			10, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// First boot: initSchema's CREATE TABLE IF NOT EXISTS is a no-op
	// (table already exists), so the new columns can only appear via the
	// ALTER-based migration.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first boot: %v", err)
	}

	newColumns := []string{
		"tokens_cached_read",
		"tokens_cached_write",
		"turn_id",
		"usage_event_id",
		"cost_source",
		"rate_input_per_million",
		"rate_cached_read_per_million",
		"rate_cached_write_per_million",
		"rate_output_per_million",
		"pricing_catalog_version",
		"cost_contract_version",
	}
	for _, col := range newColumns {
		var got *string
		if err := db.QueryRow(`SELECT ` + col + ` FROM office_cost_events WHERE id = 'legacy-1'`).Scan(&got); err != nil {
			t.Fatalf("select %s on legacy row: %v", col, err)
		}
		if got != nil {
			t.Errorf("legacy row %s = %v, want NULL (never 0)", col, *got)
		}
	}

	// tokens_cached_in itself must be untouched by the migration: it stays
	// the pre-existing sum, not backfilled or reinterpreted.
	var tokensCachedIn int64
	if err := db.QueryRow(`SELECT tokens_cached_in FROM office_cost_events WHERE id = 'legacy-1'`).Scan(&tokensCachedIn); err != nil {
		t.Fatalf("select tokens_cached_in: %v", err)
	}
	if tokensCachedIn != 40 {
		t.Errorf("tokens_cached_in = %d, want 40 (unchanged legacy sum)", tokensCachedIn)
	}

	var indexName string
	if err := db.Get(&indexName, `
		SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'uniq_office_cost_usage_event'
	`); err != nil {
		t.Fatalf("uniq_office_cost_usage_event index is missing: %v", err)
	}

	// Second boot: the migration must replay without error (idempotent
	// ALTERs) and must not disturb the already-NULL legacy row.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second boot (replay): %v", err)
	}
	var tokensCachedInAfterReplay int64
	if err := db.QueryRow(`SELECT tokens_cached_in FROM office_cost_events WHERE id = 'legacy-1'`).Scan(&tokensCachedInAfterReplay); err != nil {
		t.Fatalf("select tokens_cached_in after replay: %v", err)
	}
	if tokensCachedInAfterReplay != 40 {
		t.Errorf("tokens_cached_in after replay = %d, want 40 (unchanged)", tokensCachedInAfterReplay)
	}
}

// TestCostEventContractMigration_UsageEventIDUniqueAllowsManyNulls confirms
// the partial unique index only constrains non-NULL usage_event_id values —
// unbounded legacy/manual rows with NULL usage_event_id must never collide.
func TestCostEventContractMigration_UsageEventIDUniqueAllowsManyNulls(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	now := time.Now().UTC()
	insertRow := func(id string, usageEventID interface{}) error {
		_, err := db.Exec(`
			INSERT INTO office_cost_events (id, cost_subcents, estimated, occurred_at, created_at, usage_event_id)
			VALUES (?, 0, 0, ?, ?, ?)
		`, id, now, now, usageEventID)
		return err
	}

	if err := insertRow("row-null-1", nil); err != nil {
		t.Fatalf("insert row-null-1: %v", err)
	}
	if err := insertRow("row-null-2", nil); err != nil {
		t.Fatalf("insert row-null-2 (NULL usage_event_id must not collide): %v", err)
	}
	if err := insertRow("row-dup-1", "usage-evt-dup"); err != nil {
		t.Fatalf("insert row-dup-1: %v", err)
	}
	if err := insertRow("row-dup-2", "usage-evt-dup"); err == nil {
		t.Fatal("expected unique violation on duplicate non-NULL usage_event_id, got nil error")
	}
}
