package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

// TestSubagentContextBackfillDoesNotRunWhenShapeMigrationFails covers AC-33c
// (added by revision 2): when the AC-33 shape migration does not leave the
// table in its end-state shape, the backfill must not run at all this pass —
// not just skip writing its keys. Before this test's fix, runMigrations()
// called migrateSubagentContextBackfill() unconditionally right after
// migrateSubagentContextExecutionIdentity(), regardless of whether the
// rebuild actually succeeded.
//
// This forces the rebuild to fail (a colliding table name makes
// recreateTable's own CREATE TABLE statement error) on a legacy-shaped
// database that also has a qualifying subagent_task message seeded, and
// asserts every part of AC-33c and AC-33a together: boot survives, the table
// is left in its pre-migration (legacy) shape, subagent_context_execution_since
// is absent, NEITHER backfill activation key is written, and the message that
// would otherwise have backfilled a row did not.
func TestSubagentContextBackfillDoesNotRunWhenShapeMigrationFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subagent-context-shape-failure.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}

	repo := &Repository{db: db, ro: db, migrate: dbutil.NewMigrateLogger(db, nil)}
	if err := repo.initSchema(); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	// Roll the table back to its pre-Amendment-1 legacy shape, matching
	// TestSubagentContextExecutionIdentityMigratesLegacyShape.
	if _, err := db.Exec(`DROP TABLE task_session_subagents`); err != nil {
		t.Fatalf("drop task_session_subagents: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE task_session_subagents (
			id                  TEXT PRIMARY KEY,
			task_session_id     TEXT NOT NULL,
			task_id             TEXT NOT NULL,
			turn_id             TEXT,
			tool_call_id        TEXT NOT NULL,
			parent_tool_call_id TEXT,
			subagent_type       TEXT,
			description         TEXT,
			agent_id            TEXT,
			child_session_id    TEXT,
			model               TEXT,
			agent_status        TEXT,
			tool_status         TEXT,
			is_async            INTEGER NOT NULL DEFAULT 0,
			total_tokens        INTEGER,
			tool_use_count      INTEGER,
			duration_ms         INTEGER,
			source              TEXT NOT NULL DEFAULT 'live',
			observed_at         TIMESTAMP NOT NULL,
			settled_at          TIMESTAMP,
			updated_at          TIMESTAMP NOT NULL,
			UNIQUE (task_session_id, tool_call_id)
		)
	`); err != nil {
		t.Fatalf("recreate legacy task_session_subagents: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key IN (
		'subagent_context_execution_since', 'subagent_context_capture_since', 'subagent_context_backfill_through'
	)`); err != nil {
		t.Fatalf("reset activation keys: %v", err)
	}

	// Pre-create the table recreateTable's rebuild targets internally, so its
	// own "CREATE TABLE task_session_subagents_new (...)" statement fails —
	// the rebuild transaction rolls back, leaving the legacy shape untouched.
	if _, err := db.Exec(`CREATE TABLE task_session_subagents_new (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed colliding table: %v", err)
	}

	// A qualifying subagent_task message: if the backfill ran despite the
	// failed shape migration, this would either backfill a row (wrong: the
	// INSERT's own conflict target/columns reference the not-yet-added
	// shape) or abort with a column/constraint error that the backfill
	// swallows into its own WARN — either way, capture_since/backfill_through
	// must remain unwritten and no row must appear.
	seedForMsgTest(t, repo, "task-shape-fail", "session-shape-fail", "turn-shape-fail")
	ts := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	seedRawSubagentMessage(t, repo, "msg-shape-fail", "session-shape-fail", "task-shape-fail", "turn-shape-fail",
		`{"tool_call_id":"tc-shape-fail","status":"completed","normalized":{"kind":"subagent_task","subagent_task":{"status":"completed"}}}`,
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations must not abort boot on this migration's failure: %v", err)
	}

	holds, err := repo.subagentContextExecutionShapeHolds()
	if err != nil {
		t.Fatalf("subagentContextExecutionShapeHolds: %v", err)
	}
	if holds {
		t.Fatal("subagentContextExecutionShapeHolds = true, want false: the rebuild was forced to fail")
	}

	if _, ok := readMetaKey(t, db, "subagent_context_execution_since"); ok {
		t.Error("subagent_context_execution_since must not be written when the shape migration failed")
	}
	if _, ok := readMetaKey(t, db, "subagent_context_capture_since"); ok {
		t.Error("subagent_context_capture_since must not be written: the backfill must not run when AC-33's shape does not hold (AC-33c)")
	}
	if _, ok := readMetaKey(t, db, "subagent_context_backfill_through"); ok {
		t.Error("subagent_context_backfill_through must not be written: the backfill must not run when AC-33's shape does not hold (AC-33c)")
	}
	if count := countSubagentContextRows(t, repo, "session-shape-fail", "tc-shape-fail"); count != 0 {
		t.Errorf("row count = %d, want 0: the backfill must not have run at all", count)
	}

	// The next boot (no colliding table this time) must retry both the shape
	// migration and the backfill, and succeed.
	if _, err := db.Exec(`DROP TABLE task_session_subagents_new`); err != nil {
		t.Fatalf("remove colliding table for retry: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("second runMigrations (retry): %v", err)
	}
	if _, ok := readMetaKey(t, db, "subagent_context_execution_since"); !ok {
		t.Error("subagent_context_execution_since must be written once the retry's shape migration succeeds")
	}
	if count := countSubagentContextRows(t, repo, "session-shape-fail", "tc-shape-fail"); count != 1 {
		t.Errorf("row count after retry = %d, want 1: the backfill must run once the shape holds", count)
	}
}
