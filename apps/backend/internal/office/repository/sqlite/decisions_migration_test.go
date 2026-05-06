package sqlite_test

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// legacyOfficeDecisionsSchema mirrors the pre-Wave-E schema for
// office_task_approval_decisions. The migration test pre-creates this
// table directly so the copy-into-workflow_step_decisions + drop
// migration has rows to act on.
const legacyOfficeDecisionsSchema = `
CREATE TABLE IF NOT EXISTS office_task_approval_decisions (
	id              TEXT NOT NULL PRIMARY KEY,
	task_id         TEXT NOT NULL,
	decider_type    TEXT NOT NULL CHECK (decider_type IN ('user','agent')),
	decider_id      TEXT NOT NULL,
	role            TEXT NOT NULL CHECK (role IN ('reviewer','approver')),
	decision        TEXT NOT NULL CHECK (decision IN ('approved','changes_requested')),
	comment         TEXT NOT NULL DEFAULT '',
	created_at      DATETIME NOT NULL,
	superseded_at   DATETIME
);
`

// minimalDecisionsTasksSchema is the slim tasks table the decisions
// migration test needs. The migration only reads tasks.workflow_step_id,
// so a single-column subset suffices.
const minimalDecisionsTasksSchema = `
CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	workflow_step_id TEXT NOT NULL DEFAULT ''
);
`

// minimalWorkflowDecisionsSchema mirrors the workflow_step_decisions
// columns used by the migration. The workflow store creates the real
// table via initPhase2Schema, but the migration test is self-contained
// (does not boot a workflow repo) so we declare the columns inline.
const minimalWorkflowDecisionsSchema = `
CREATE TABLE IF NOT EXISTS workflow_step_decisions (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	step_id TEXT NOT NULL,
	participant_id TEXT NOT NULL,
	decision TEXT NOT NULL,
	note TEXT DEFAULT '',
	decided_at TIMESTAMP NOT NULL,
	superseded_at TIMESTAMP NULL,
	decider_type TEXT NOT NULL DEFAULT '',
	decider_id TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT '',
	comment TEXT NOT NULL DEFAULT ''
);
`

// minimalWorkflowParticipantsSchema mirrors workflow_step_participants.
// Used so the migration's participant lookup has a target.
const minimalWorkflowParticipantsSchema = `
CREATE TABLE IF NOT EXISTS workflow_step_participants (
	id TEXT PRIMARY KEY,
	step_id TEXT NOT NULL,
	task_id TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	agent_profile_id TEXT NOT NULL,
	decision_required INTEGER NOT NULL DEFAULT 0,
	position INTEGER NOT NULL DEFAULT 0
);
`

// TestMigrateOfficeDecisionsToWorkflow verifies that the Wave E migration
// copies legacy office_task_approval_decisions rows into
// workflow_step_decisions, resolving the participant via
// workflow_step_participants for agents and the "user" sentinel for
// human decisions, then drops the legacy table.
func TestMigrateOfficeDecisionsToWorkflow(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings provide: %v", err)
	}

	for _, ddl := range []string{
		minimalDecisionsTasksSchema,
		legacyOfficeDecisionsSchema,
		minimalWorkflowDecisionsSchema,
		minimalWorkflowParticipantsSchema,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create schema: %v", err)
		}
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

	// Seed an agent participant so the migration can resolve participant_id
	// for the agent decision.
	if _, err := db.Exec(`INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('part-1', 'step-1', 'task-with-step', 'approver', 'agent-A', 1, 0)`,
	); err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	// Seed legacy decisions: an agent approver, a user approver, and an
	// orphan whose task has no step (must be skipped).
	if _, err := db.Exec(`
		INSERT INTO office_task_approval_decisions
			(id, task_id, decider_type, decider_id, role, decision, comment, created_at)
		VALUES
			('d-agent', 'task-with-step', 'agent', 'agent-A', 'approver', 'approved', 'lgtm', datetime('now')),
			('d-user',  'task-with-step', 'user',  'user',    'approver', 'approved', '',     datetime('now')),
			('d-orphan','task-no-step',   'agent', 'agent-X', 'approver', 'approved', '',     datetime('now'))
	`); err != nil {
		t.Fatalf("seed legacy decisions: %v", err)
	}

	// NewWithDB runs runMigrations; the decisions migration follows the
	// participants migration. After this, office_task_approval_decisions
	// is gone and workflow_step_decisions holds the resolvable rows.
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	// Verify the legacy table is gone.
	var legacy int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='office_task_approval_decisions'`,
	).Scan(&legacy); err != nil {
		t.Fatalf("query legacy: %v", err)
	}
	if legacy != 0 {
		t.Errorf("legacy office_task_approval_decisions still present after migration")
	}

	// Verify the agent decision migrated with the resolved participant_id.
	var (
		gotPart, gotStep, gotRole, gotDecider string
	)
	if err := db.QueryRow(
		`SELECT participant_id, step_id, role, decider_id FROM workflow_step_decisions WHERE id = 'd-agent'`,
	).Scan(&gotPart, &gotStep, &gotRole, &gotDecider); err != nil {
		t.Fatalf("read d-agent: %v", err)
	}
	if gotPart != "part-1" || gotStep != "step-1" || gotRole != "approver" || gotDecider != "agent-A" {
		t.Errorf("d-agent fields = %q/%q/%q/%q, want part-1/step-1/approver/agent-A",
			gotPart, gotStep, gotRole, gotDecider)
	}

	// Verify the user decision migrated with the user sentinel participant_id.
	var userPart string
	if err := db.QueryRow(
		`SELECT participant_id FROM workflow_step_decisions WHERE id = 'd-user'`,
	).Scan(&userPart); err != nil {
		t.Fatalf("read d-user: %v", err)
	}
	if userPart != "user" {
		t.Errorf("d-user participant_id = %q, want 'user'", userPart)
	}

	// Verify the orphan decision is NOT in the workflow table (no step).
	var orphan int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM workflow_step_decisions WHERE id = 'd-orphan'`,
	).Scan(&orphan); err != nil {
		t.Fatalf("count d-orphan: %v", err)
	}
	if orphan != 0 {
		t.Errorf("orphan migrated unexpectedly: count = %d", orphan)
	}

	// Idempotency: re-init must be a no-op.
	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("idempotent re-init: %v", err)
	}
	var total int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM workflow_step_decisions`,
	).Scan(&total); err != nil {
		t.Fatalf("count after re-init: %v", err)
	}
	if total != 2 {
		t.Errorf("idempotency violated: rows = %d, want 2", total)
	}
}

// TestMigrateOfficeDecisionsToWorkflow_NoLegacyTable verifies that when the
// legacy table is absent on a fresh install, the migration short-circuits
// cleanly.
func TestMigrateOfficeDecisionsToWorkflow_NoLegacyTable(t *testing.T) {
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

// TestMigrateOfficeDecisionsToWorkflow_PreservesSupersededAt ensures the
// migration carries the superseded_at timestamp so the audit history is
// preserved when the legacy table held both the latest and prior rows.
func TestMigrateOfficeDecisionsToWorkflow_PreservesSupersededAt(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings provide: %v", err)
	}
	for _, ddl := range []string{
		minimalDecisionsTasksSchema,
		legacyOfficeDecisionsSchema,
		minimalWorkflowDecisionsSchema,
		minimalWorkflowParticipantsSchema,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO tasks (id, workflow_step_id) VALUES ('t1', 'step-1')`,
	); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('p1', 'step-1', 't1', 'approver', 'a', 1, 0)`); err != nil {
		t.Fatalf("seed participant: %v", err)
	}
	// First decision was superseded by a later one (both rows preserved).
	if _, err := db.Exec(`
		INSERT INTO office_task_approval_decisions
			(id, task_id, decider_type, decider_id, role, decision, comment, created_at, superseded_at)
		VALUES
			('first',  't1', 'agent', 'a', 'approver', 'approved', '', datetime('now','-1 hour'), datetime('now','-30 minutes')),
			('second', 't1', 'agent', 'a', 'approver', 'changes_requested', 'fix it', datetime('now','-30 minutes'), NULL)
	`); err != nil {
		t.Fatalf("seed decisions: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db); err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	var firstSuperseded int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM workflow_step_decisions WHERE id='first' AND superseded_at IS NOT NULL`,
	).Scan(&firstSuperseded); err != nil {
		t.Fatalf("query first: %v", err)
	}
	if firstSuperseded != 1 {
		t.Errorf("first decision superseded_at not preserved")
	}
	var secondActive int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM workflow_step_decisions WHERE id='second' AND superseded_at IS NULL`,
	).Scan(&secondActive); err != nil {
		t.Fatalf("query second: %v", err)
	}
	if secondActive != 1 {
		t.Errorf("second decision must be active (superseded_at IS NULL)")
	}
}
