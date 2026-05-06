package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// legacyOfficeTaskParticipantsSchema mirrors the pre-Wave-C schema for the
// office_task_participants table. The migration test pre-creates this
// table directly so the copy-into-workflow_step_participants + drop
// migration has rows to act on.
const legacyOfficeTaskParticipantsSchema = `
CREATE TABLE IF NOT EXISTS office_task_participants (
	task_id            TEXT NOT NULL,
	agent_profile_id   TEXT NOT NULL,
	role               TEXT NOT NULL CHECK (role IN ('reviewer','approver')),
	created_at         DATETIME NOT NULL,
	PRIMARY KEY (task_id, agent_profile_id, role)
);
`

// minimalTasksSchema is the slim tasks table the migration test needs.
// We don't use the office repo's real tasks creation because the office
// schema doesn't own that table — production wires it via
// internal/task/repository/sqlite. The migration only reads
// tasks.workflow_step_id, so a single-column subset suffices.
const minimalTasksSchema = `
CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	workflow_step_id TEXT NOT NULL DEFAULT ''
);
`

// TestMigrateOfficeTaskParticipantsToWorkflow verifies that the Wave C
// migration copies legacy office_task_participants rows into
// workflow_step_participants under each task's workflow_step_id, then
// drops the legacy table. Rows with no resolvable step are silently
// skipped, matching the runtime fallback.
func TestMigrateOfficeTaskParticipantsToWorkflow(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings provide: %v", err)
	}

	// Pre-create the tables BEFORE the office repo runs migrations.
	if _, err := db.Exec(minimalTasksSchema); err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	if _, err := db.Exec(legacyOfficeTaskParticipantsSchema); err != nil {
		t.Fatalf("create legacy participants: %v", err)
	}
	// workflow_step_participants is owned by the workflow store in
	// production; the migration test creates it inline so the copy has
	// somewhere to land.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL,
			task_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL,
			agent_profile_id TEXT NOT NULL,
			decision_required INTEGER NOT NULL DEFAULT 0,
			position INTEGER NOT NULL DEFAULT 0
		)`); err != nil {
		t.Fatalf("create workflow_step_participants: %v", err)
	}

	// Seed two tasks: one with a step (migratable), one without (skipped).
	if _, err := db.Exec(
		`INSERT INTO tasks (id, workflow_step_id) VALUES ('task-with-step', 'step-1')`,
	); err != nil {
		t.Fatalf("seed task with step: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO tasks (id, workflow_step_id) VALUES ('task-no-step', '')`,
	); err != nil {
		t.Fatalf("seed task without step: %v", err)
	}

	// Seed three legacy participant rows.
	if _, err := db.Exec(`
		INSERT INTO office_task_participants (task_id, agent_profile_id, role, created_at)
		VALUES ('task-with-step', 'agent-rev', 'reviewer', datetime('now')),
		       ('task-with-step', 'agent-app', 'approver', datetime('now')),
		       ('task-no-step',   'agent-orphan', 'reviewer', datetime('now'))
	`); err != nil {
		t.Fatalf("seed legacy participants: %v", err)
	}

	// NewWithDB runs runMigrations; the participants migration follows the
	// agent migration. After this, office_task_participants is gone and
	// workflow_step_participants holds the migrated rows under step-1.
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	// Verify the legacy table is gone.
	var legacy int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='office_task_participants'`,
	).Scan(&legacy); err != nil {
		t.Fatalf("query legacy: %v", err)
	}
	if legacy != 0 {
		t.Errorf("legacy office_task_participants still present after migration")
	}

	// Verify the rows under step-1 are present (orphan dropped).
	var migrated int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM workflow_step_participants WHERE step_id = 'step-1' AND task_id = 'task-with-step'`,
	).Scan(&migrated); err != nil {
		t.Fatalf("count migrated: %v", err)
	}
	if migrated != 2 {
		t.Errorf("migrated rows = %d, want 2", migrated)
	}

	// Idempotency: a second NewWithDB on the post-drop schema must be a no-op.
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("idempotent re-init: %v", err)
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM workflow_step_participants WHERE step_id = 'step-1' AND task_id = 'task-with-step'`,
	).Scan(&migrated); err != nil {
		t.Fatalf("count re-init: %v", err)
	}
	if migrated != 2 {
		t.Errorf("idempotency violated: rows = %d, want 2", migrated)
	}
}

// TestMigrateOfficeTaskParticipantsToWorkflow_NoLegacyTable verifies that
// when the legacy table is absent on a fresh install, the migration
// short-circuits cleanly.
func TestMigrateOfficeTaskParticipantsToWorkflow_NoLegacyTable(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings provide: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("office repo on fresh DB: %v", err)
	}
}
