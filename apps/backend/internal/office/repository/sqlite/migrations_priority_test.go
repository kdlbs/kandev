package sqlite_test

import (
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestMigrate_PriorityIntegerToText verifies the office migration converts an
// existing tasks table with INTEGER priority into a TEXT priority column with
// the four-value CHECK constraint, mapping all existing values to 'medium'.
func TestMigrate_PriorityIntegerToText(t *testing.T) {
	dbPath := t.TempDir() + "/test.db?_journal_mode=WAL"
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Create a legacy tasks table with INTEGER priority and pre-populated rows.
	if _, err := db.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			is_ephemeral INTEGER NOT NULL DEFAULT 0,
			parent_id TEXT DEFAULT '',
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			assignee_agent_profile_id TEXT DEFAULT '',
			origin TEXT DEFAULT 'manual',
			project_id TEXT DEFAULT '',
			requires_approval INTEGER DEFAULT 0,
			execution_policy TEXT DEFAULT '',
			execution_state TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT,
			checkout_agent_id TEXT,
			checkout_at DATETIME
		);
		INSERT INTO tasks (id, title, priority, created_at, updated_at) VALUES ('t1', 'A', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO tasks (id, title, priority, created_at, updated_at) VALUES ('t2', 'B', 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO tasks (id, title, priority, created_at, updated_at) VALUES ('t3', 'C', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("init office repo (run migrations): %v", err)
	}

	// Column type should now be TEXT.
	rows, err := db.Queryx(`PRAGMA table_info(tasks)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer func() { _ = rows.Close() }()

	gotType := ""
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt *string
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma: %v", err)
		}
		if name == "priority" {
			gotType = ctype
			break
		}
	}
	if !strings.EqualFold(gotType, "TEXT") {
		t.Fatalf("priority column type after migration = %q, want TEXT", gotType)
	}

	// Existing rows must survive with priority remapped to 'medium'.
	type row struct {
		ID       string `db:"id"`
		Priority string `db:"priority"`
	}
	var got []row
	if err := db.Select(&got, `SELECT id, priority FROM tasks ORDER BY id`); err != nil {
		t.Fatalf("select after migration: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("rows after migration = %d, want 3", len(got))
	}
	for _, r := range got {
		if r.Priority != "medium" {
			t.Errorf("row %s priority = %q, want medium", r.ID, r.Priority)
		}
	}

	// CHECK constraint should reject invalid values.
	if _, err := db.Exec(`INSERT INTO tasks (id, title, priority, created_at, updated_at) VALUES ('bad', 'X', 'urgent', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err == nil {
		t.Error("expected CHECK constraint to reject invalid priority")
	}

	// Valid enum values are accepted.
	for _, ok := range []string{"critical", "high", "medium", "low"} {
		if _, err := db.Exec(`INSERT INTO tasks (id, title, priority, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, "ok-"+ok, "T", ok); err != nil {
			t.Errorf("priority %q rejected: %v", ok, err)
		}
	}
}

// TestMigrate_PriorityIdempotent verifies running the office init twice over
// an already-migrated tasks table is a no-op.
func TestMigrate_PriorityIdempotent(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			priority TEXT NOT NULL DEFAULT 'medium'
				CHECK (priority IN ('critical','high','medium','low')),
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO tasks (id, title, priority) VALUES ('t1', 'A', 'high');
	`); err != nil {
		t.Fatalf("seed migrated schema: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}

	var p string
	if err := db.QueryRow(`SELECT priority FROM tasks WHERE id='t1'`).Scan(&p); err != nil {
		t.Fatalf("read after idempotent init: %v", err)
	}
	if p != "high" {
		t.Errorf("priority preserved = %q, want high", p)
	}
}

// TestParticipantsTable_Created — REMOVED in ADR 0005 Wave C.
// office_task_participants was dropped; reviewer/approver rows now live
// in workflow_step_participants (created and validated in
// internal/workflow/repository/phase2_sqlite.go and its tests).
