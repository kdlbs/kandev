package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// TestPostgresAddTaskParticipant_ClaimDecisionGuard_RolelessDecisionCommitsInWindow
// is the cross-transaction reading of AC-OFFICE-SEAT-GUARD-002.2: the decision
// genuinely COMMITS from another connection inside the registration's window,
// rather than being written through the registration's own transaction as the
// engine-agnostic test does.
//
// It cannot deadlock. A roleless decision skips ParticipantRoleSeatLockKey -
// which the registration holds for its whole transaction - and takes only
// decisionLockNamespace, which the registration never touches. That skip is
// exactly why this window is reachable at all.
//
// The embedded engine cannot host this test: the office sqlite suite pins the
// pool to one connection, and SQLite serializes writers regardless, so a second
// connection would block behind the open registration transaction rather than
// commit inside it.
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAddTaskParticipant_ClaimDecisionGuard_RolelessDecisionCommitsInWindow(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConnForClaimRace(t, dsn, 4)
	ctx := context.Background()

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	workflowRepo, err := workflowrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init workflow repo: %v", err)
	}
	officeRepo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES ('wf-guard-window', '', 'Guard Window', ?, ?)
	`), now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	step := &models.WorkflowStep{
		WorkflowID: "wf-guard-window", Name: "Review", Position: 0, StageType: models.StageTypeReview,
	}
	if err := workflowRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	for _, agentID := range []string{"agent-auto", "agent-registering"} {
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, '', '', ?, ?)
		`), agentID, agentID, agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workflow_step_id, title, created_at, updated_at)
		VALUES ('task-guard-window', ?, 'guard', ?, ?)
	`), step.ID, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, provenance)
		VALUES ('seat-guard-window', ?, 'task-guard-window', 'reviewer', 'agent-auto', 1, 0, 'auto')
	`), step.ID); err != nil {
		t.Fatalf("seed auto seat: %v", err)
	}

	// The hook runs inside the registration's open transaction; this commits
	// on a different pooled connection, so the decision is durable before the
	// reassigning UPDATE evaluates its condition.
	var hookErr error
	restore := sqlite.SetClaimWindowHook(func(hookCtx context.Context, _ *sqlx.Tx, seatID string) {
		if seatID != "seat-guard-window" {
			return
		}
		hookErr = workflowRepo.RecordStepDecision(hookCtx, &models.WorkflowStepDecision{
			TaskID: "task-guard-window", StepID: step.ID, ParticipantID: seatID,
			Decision: "approved", DeciderID: "agent-auto",
		})
	})
	defer restore()

	result, err := officeRepo.AddTaskParticipant(ctx, "task-guard-window", "agent-registering", "reviewer")
	if err != nil {
		t.Fatalf("AddTaskParticipant: %v", err)
	}
	if hookErr != nil {
		t.Fatalf("roleless decision must commit inside the window: %v", hookErr)
	}

	if result.Outcome != sqlite.ParticipantWriteOutcomeInserted {
		t.Errorf("outcome = %v, want Inserted", result.Outcome)
	}
	if result.DisplacedAgentProfileID != "" {
		t.Errorf("displaced = %q, want empty", result.DisplacedAgentProfileID)
	}

	var seatAgent string
	if err := db.GetContext(ctx, &seatAgent, db.Rebind(
		`SELECT agent_profile_id FROM workflow_step_participants WHERE id = 'seat-guard-window'`,
	)); err != nil {
		t.Fatalf("read seat: %v", err)
	}
	if seatAgent != "agent-auto" {
		t.Errorf("seat agent = %q, want agent-auto: the decision was reattributed", seatAgent)
	}
}
