package sqlite_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestMigrate_RoutineCatchUpPolicyDefaultRebuild seeds a legacy
// office_routines table (as it existed before this change: DEFAULT
// 'enqueue_missed_with_cap', no gap-summary columns on office_routine_runs)
// with a routine, a trigger, and a run already attached, then asserts the
// office repo migration:
//   - rewrites the stored DEFAULT to 'summarize_missed' (AC-003.3, visible
//     on the live schema of an upgraded install, not just in a fresh one)
//   - normalizes the pre-existing 'enqueue_missed_with_cap' row value
//   - does not cascade-delete the routine's trigger or run rows, which is
//     the hazard PRAGMA foreign_keys=OFF exists to avoid during the
//     drop+rename table-rebuild dance
//   - preserves every column and timestamp on the surviving rows
func TestMigrate_RoutineCatchUpPolicyDefaultRebuild(t *testing.T) {
	dbPath := t.TempDir() + "/test.db?_journal_mode=WAL"
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`
		CREATE TABLE office_routines (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			task_template TEXT NOT NULL DEFAULT '{}',
			assignee_agent_profile_id TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			concurrency_policy TEXT DEFAULT 'skip_if_active',
			catch_up_policy TEXT NOT NULL DEFAULT 'enqueue_missed_with_cap',
			catch_up_max INTEGER NOT NULL DEFAULT 25,
			variables TEXT DEFAULT '{}',
			last_run_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE office_routine_triggers (
			id TEXT PRIMARY KEY,
			routine_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			cron_expression TEXT DEFAULT '',
			timezone TEXT DEFAULT '',
			public_id TEXT DEFAULT '',
			signing_mode TEXT DEFAULT '',
			secret TEXT DEFAULT '',
			next_run_at TIMESTAMP,
			last_fired_at TIMESTAMP,
			enabled INTEGER DEFAULT 1,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE
		);
		CREATE TABLE office_routine_runs (
			id TEXT PRIMARY KEY,
			routine_id TEXT NOT NULL,
			trigger_id TEXT DEFAULT '',
			source TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'received',
			trigger_payload TEXT DEFAULT '{}',
			linked_task_id TEXT DEFAULT '',
			coalesced_into_run_id TEXT DEFAULT '',
			dispatch_fingerprint TEXT DEFAULT '',
			started_at TIMESTAMP,
			completed_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE
		);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO office_routines (
			id, workspace_id, name, description, task_template,
			assignee_agent_profile_id, status, concurrency_policy,
			catch_up_policy, catch_up_max, variables, last_run_at, created_at, updated_at
		) VALUES (
			'routine-legacy', 'ws-1', 'Legacy routine', 'desc', '{}',
			'agent-1', 'active', 'skip_if_active',
			'enqueue_missed_with_cap', 25, '{}', NULL, ?, ?
		)`, createdAt, createdAt); err != nil {
		t.Fatalf("seed legacy routine: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO office_routine_triggers (
			id, routine_id, kind, cron_expression, timezone, enabled, created_at, updated_at
		) VALUES ('trigger-legacy', 'routine-legacy', 'cron', '* * * * *', 'UTC', 1, ?, ?)
	`, createdAt, createdAt); err != nil {
		t.Fatalf("seed legacy trigger: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO office_routine_runs (
			id, routine_id, trigger_id, source, status, trigger_payload, created_at
		) VALUES ('run-legacy', 'routine-legacy', 'trigger-legacy', 'cron', 'received', '{}', ?)
	`, createdAt); err != nil {
		t.Fatalf("seed legacy run: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init office repo (run migrations): %v", err)
	}

	// The stored DEFAULT clause itself must now read summarize_missed.
	rows, err := db.Queryx(`PRAGMA table_info(office_routines)`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()
	dfltFound := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma: %v", err)
		}
		if name == "catch_up_policy" {
			dfltFound = true
			if strings.Contains(dflt.String, "enqueue_missed_with_cap") {
				t.Fatalf("catch_up_policy DEFAULT still names the retired value: %q", dflt.String)
			}
			if !strings.Contains(dflt.String, "summarize_missed") {
				t.Fatalf("catch_up_policy DEFAULT = %q, want it to contain summarize_missed", dflt.String)
			}
		}
	}
	if !dfltFound {
		t.Fatal("catch_up_policy column not found after migration")
	}

	// The pre-existing row's value is normalized.
	var policy string
	var routineCreatedAt, routineUpdatedAt time.Time
	if err := db.QueryRow(
		`SELECT catch_up_policy, created_at, updated_at FROM office_routines WHERE id = 'routine-legacy'`,
	).Scan(&policy, &routineCreatedAt, &routineUpdatedAt); err != nil {
		t.Fatalf("select migrated routine: %v", err)
	}
	if policy != "summarize_missed" {
		t.Errorf("routine-legacy catch_up_policy = %q, want summarize_missed", policy)
	}
	if !routineCreatedAt.Equal(createdAt) {
		t.Errorf("routine-legacy created_at = %v, want %v (preserved across rebuild)", routineCreatedAt, createdAt)
	}

	// The trigger and run rows must survive the drop+rename — this is the
	// cascade PRAGMA foreign_keys=OFF exists to prevent.
	var triggerCount, runCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM office_routine_triggers WHERE id = 'trigger-legacy'`).Scan(&triggerCount); err != nil {
		t.Fatalf("count trigger: %v", err)
	}
	if triggerCount != 1 {
		t.Errorf("trigger-legacy survived = %d rows, want 1 (cascaded away by the rebuild)", triggerCount)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM office_routine_runs WHERE id = 'run-legacy'`).Scan(&runCount); err != nil {
		t.Fatalf("count run: %v", err)
	}
	if runCount != 1 {
		t.Errorf("run-legacy survived = %d rows, want 1 (cascaded away by the rebuild)", runCount)
	}

	// Re-running the migration is a no-op: the guard sees a non-stale
	// default and skips the rebuild.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM office_routines WHERE id = 'routine-legacy'`).Scan(&runCount); err != nil {
		t.Fatalf("count routine after replay: %v", err)
	}
	if runCount != 1 {
		t.Errorf("routine-legacy count after replay = %d, want 1", runCount)
	}
}
