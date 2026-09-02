package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// TestGetTaskAssignee_LegacyEpochRowsResolveToLaterInserted reproduces the
// upgrade scenario RunnerProjection's task-scoped fallback (base.go) must
// still get right: two workflow_step_participants rows written before the
// created_at column existed both carry the ADD COLUMN default
// ('1970-01-01 00:00:00'), so they tie on `ORDER BY created_at DESC`. Without
// workflowrepo's backfill (phase2_sqlite.go,
// backfillParticipantsCreatedAtFromRowid), the `wsp.id ASC` tiebreak would
// pick an arbitrary UUID instead of the most recently assigned runner —
// what `ORDER BY rowid DESC` returned before this fallback was made
// Postgres-portable.
//
// Uses the real task + workflow + office repositories on one database (the
// office package's own test schema stubs workflow_step_participants
// directly and never runs workflowrepo's migrations, so it can't observe
// the backfill). Rows are inserted directly via SQL with created_at forced
// to the legacy default, then workflowrepo.NewWithDB is called a second
// time to rerun initPhase2Schema against the now-existing tied rows, the
// same as a backend restart after upgrade.
func TestGetTaskAssignee_LegacyEpochRowsResolveToLaterInserted(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()

	taskRepo, err := taskrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	workflowRepo, err := workflowrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init workflow repo (first boot): %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	ctx := context.Background()
	var wsID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM workspaces LIMIT 1`).Scan(&wsID); err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workflowID, err := taskRepo.EnsureOfficeWorkflow(ctx, wsID)
	if err != nil {
		t.Fatalf("ensure workflow: %v", err)
	}
	var currentStepID string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM workflow_steps WHERE workflow_id = ? AND is_start_step = 1`,
		workflowID).Scan(&currentStepID); err != nil {
		t.Fatalf("get start step: %v", err)
	}

	// A second, distinct step the legacy runner rows attach to: the task's
	// *current* step must have neither a per-step runner row nor a step
	// primary, so RunnerProjection falls through to the third,
	// task-scoped-across-any-step arm this test targets.
	otherStep := &models.WorkflowStep{WorkflowID: workflowID, Name: "Other", Position: 99}
	if err := workflowRepo.CreateStep(ctx, otherStep); err != nil {
		t.Fatalf("create other step: %v", err)
	}

	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'Legacy task', datetime('now'), datetime('now'))
	`), "task-legacy", wsID, workflowID, currentStepID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	// Both rows left at the ADD COLUMN default, inserted oldest-first so
	// agent-newer gets the higher rowid. IDs are deliberately ordered
	// opposite to insertion order (id-1 < id-2 lexically, but id-1 is the
	// *later* insert): without the created_at backfill, both rows tie and
	// the id-ASC tiebreak would pick id-1 (agent-older) — the wrong,
	// pre-fix answer — so this can't pass by id-ordering coincidence.
	insertLegacyRunner := func(id, agentProfileID string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
			VALUES (?, ?, ?, 'runner', ?, 0, 0, '1970-01-01 00:00:00')
		`), id, otherStep.ID, "task-legacy", agentProfileID); err != nil {
			t.Fatalf("insert legacy runner %s: %v", id, err)
		}
	}
	insertLegacyRunner("id-1", "agent-older")
	insertLegacyRunner("id-2", "agent-newer")

	// Simulate a backend restart on the upgraded binary: rerunning
	// initPhase2Schema is what backfills the now-existing tied rows.
	if _, err := workflowrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init workflow repo (second boot): %v", err)
	}

	got, err := officeRepo.GetTaskAssignee(ctx, "task-legacy")
	if err != nil {
		t.Fatalf("GetTaskAssignee: %v", err)
	}
	if got != "agent-newer" {
		t.Fatalf("GetTaskAssignee = %q, want agent-newer (later-inserted legacy row, matching pre-fix rowid DESC behaviour)", got)
	}
}
