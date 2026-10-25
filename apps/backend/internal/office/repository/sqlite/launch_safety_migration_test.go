package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// legacyRunsTableDDL is the runs table shape as it existed before the
// causation/priority/actor/workspace columns were added: every column
// createRunTables declares up to (and not including) the launch-safety
// block. A database on this shape is what every existing installation
// looks like the first time it boots this branch.
const legacyRunsTableDDL = `
CREATE TABLE runs (
	id TEXT PRIMARY KEY,
	agent_profile_id TEXT NOT NULL,
	reason TEXT NOT NULL,
	payload TEXT DEFAULT '{}',
	status TEXT NOT NULL DEFAULT 'queued',
	coalesced_count INTEGER DEFAULT 1,
	idempotency_key TEXT,
	context_snapshot TEXT DEFAULT '{}',
	capabilities TEXT NOT NULL DEFAULT '{}',
	input_snapshot TEXT NOT NULL DEFAULT '{}',
	output_summary TEXT NOT NULL DEFAULT '',
	failure_reason TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	retry_count INTEGER DEFAULT 0,
	scheduled_retry_at TIMESTAMP,
	error_message TEXT NOT NULL DEFAULT '',
	cancel_reason TEXT,
	outcome TEXT,
	logical_provider_order TEXT,
	requested_tier TEXT,
	resolved_execution_profile_id TEXT,
	resolved_provider_id TEXT,
	resolved_model TEXT,
	current_route_attempt_seq INTEGER NOT NULL DEFAULT 0,
	routing_blocked_status TEXT,
	earliest_retry_at TIMESTAMP,
	route_cycle_baseline_seq INTEGER NOT NULL DEFAULT 0,
	result_json TEXT NOT NULL DEFAULT '{}',
	assembled_prompt TEXT NOT NULL DEFAULT '',
	summary_injected TEXT NOT NULL DEFAULT '',
	continuation_scope TEXT NOT NULL DEFAULT '',
	requested_at TIMESTAMP NOT NULL,
	claimed_at TIMESTAMP,
	finished_at TIMESTAMP
);
CREATE INDEX idx_run_status_requested ON runs(status, requested_at);
CREATE UNIQUE INDEX idx_run_idempotency ON runs(idempotency_key) WHERE idempotency_key IS NOT NULL;
`

// TestBoot_ExistingDatabaseWithoutLaunchSafetyColumns_MigratesWithoutError
// covers an existing (pre-this-capability) database's first boot on this
// branch: runs already exists without the nine causation/priority/actor/
// workspace columns, so CREATE TABLE IF NOT EXISTS runs is a no-op and
// those columns only appear via the ADD COLUMN migrations that run later
// in runMigrations. A statement that references one of them before that
// point — such as an index in the same batch as the no-op CREATE TABLE —
// fails the whole boot with "no such column" on every such installation.
func TestBoot_ExistingDatabaseWithoutLaunchSafetyColumns_MigratesWithoutError(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(legacyRunsTableDDL); err != nil {
		t.Fatalf("seed legacy runs table: %v", err)
	}

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("boot against a pre-existing legacy-shaped runs table: %v", err)
	}

	var causationCol string
	if err := db.Get(&causationCol, `SELECT name FROM pragma_table_info('runs') WHERE name = 'causation_id'`); err != nil {
		t.Fatalf("causation_id column missing after migration: %v", err)
	}
}
