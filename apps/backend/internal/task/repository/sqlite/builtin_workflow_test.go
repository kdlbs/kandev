package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// newRepoForBuiltinWorkflowTests builds a fresh repo against an on-disk SQLite
// file. The Phase 6 (ADR-0004) ensure helpers seed workflows + steps via the
// task repository, so the test exercises the full schema migration chain.
func newRepoForBuiltinWorkflowTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "builtin-wf.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	// The Phase 6 ensure helpers write into workflow_steps, whose schema
	// is initialised by the workflow repository. Build it on the same DB
	// so tests exercise the production migration order.
	if _, err := workflowrepo.NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("init workflow repo schema: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

// loadStepsForWorkflow returns the workflow_steps rows for a workflow,
// ordered by position. Each row is parsed into a wfmodels.WorkflowStep so
// tests can introspect events.
func loadStepsForWorkflow(t *testing.T, repo *Repository, workflowID string) []*wfmodels.WorkflowStep {
	t.Helper()
	rows, err := repo.db.QueryContext(context.Background(), repo.db.Rebind(`
		SELECT id, name, position, stage_type, events, auto_advance_requires_signal
		FROM workflow_steps WHERE workflow_id = ? ORDER BY position
	`), workflowID)
	if err != nil {
		t.Fatalf("query steps: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*wfmodels.WorkflowStep
	for rows.Next() {
		step := &wfmodels.WorkflowStep{WorkflowID: workflowID}
		var stage, events string
		var autoAdvanceRequiresSignal int
		if err := rows.Scan(&step.ID, &step.Name, &step.Position, &stage, &events, &autoAdvanceRequiresSignal); err != nil {
			t.Fatalf("scan step: %v", err)
		}
		step.AutoAdvanceRequiresSignal = autoAdvanceRequiresSignal != 0
		step.StageType = wfmodels.StageType(stage)
		if events != "" {
			if err := json.Unmarshal([]byte(events), &step.Events); err != nil {
				t.Fatalf("unmarshal events: %v", err)
			}
		}
		out = append(out, step)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

func assertBuiltinWorkflowHidden(t *testing.T, repo *Repository, workflowID string) {
	t.Helper()
	var hidden int
	if err := repo.db.QueryRowContext(
		context.Background(),
		repo.db.Rebind(`SELECT hidden FROM workflows WHERE id = ?`),
		workflowID,
	).Scan(&hidden); err != nil {
		t.Fatalf("query workflow visibility: %v", err)
	}
	if hidden != 1 {
		t.Errorf("workflow %q hidden = %d, want 1", workflowID, hidden)
	}
}

func TestEnsureOfficeDefaultWorkflow_CreatesFiveSteps(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()

	id, err := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ensure office-default: %v", err)
	}
	if id == "" {
		t.Fatalf("expected non-empty workflow id")
	}
	assertBuiltinWorkflowHidden(t, repo, id)

	steps := loadStepsForWorkflow(t, repo, id)
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}
	expected := []struct {
		Name      string
		StageType wfmodels.StageType
	}{
		{"Backlog", wfmodels.StageTypeCustom},
		{"Work", wfmodels.StageTypeWork},
		{"In Review", wfmodels.StageTypeReview}, // overridden below
		{"Approval", wfmodels.StageTypeApproval},
		{"Done", wfmodels.StageTypeCustom},
	}
	// Match the YAML's actual step name "Review" (not "In Review").
	expected[2].Name = "Review"

	for i, want := range expected {
		if steps[i].Name != want.Name {
			t.Errorf("step %d name = %q, want %q", i, steps[i].Name, want.Name)
		}
		if steps[i].StageType != want.StageType {
			t.Errorf("step %d stage_type = %q, want %q", i, steps[i].StageType, want.StageType)
		}
	}

	work := findStepByNameLocal(steps, "Work")
	if work == nil {
		t.Fatal("missing Work step")
	}
	if !work.AutoAdvanceRequiresSignal {
		t.Error("Work.AutoAdvanceRequiresSignal = false, want true (ADR-0015 completion signal gate)")
	}
	for _, name := range []string{"Backlog", "Review", "Approval", "Done"} {
		if step := findStepByNameLocal(steps, name); step != nil && step.AutoAdvanceRequiresSignal {
			t.Errorf("%s.AutoAdvanceRequiresSignal = true, want false", name)
		}
	}
}

func TestCreateWorkspaceWithKanban_CancelTriggersTurnCompleteDefaults(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	workflow, err := repo.CreateWorkspaceWithKanban(ctx, &taskmodels.Workspace{ID: "ws-kanban-cancel", Name: "Kanban Cancel"})
	if err != nil {
		t.Fatalf("create Kanban workspace: %v", err)
	}

	rows, err := repo.db.QueryContext(ctx, repo.db.Rebind(`
		SELECT name, cancel_triggers_turn_complete
		FROM workflow_steps WHERE workflow_id = ? ORDER BY position
	`), workflow.ID)
	if err != nil {
		t.Fatalf("query Kanban flags: %v", err)
	}
	defer func() { _ = rows.Close() }()
	want := map[string]bool{"Backlog": true, "In Progress": true, "Review": false, "Done": false}
	seen := 0
	for rows.Next() {
		var name string
		var enabled int
		if err := rows.Scan(&name, &enabled); err != nil {
			t.Fatalf("scan Kanban flag: %v", err)
		}
		if wantValue, ok := want[name]; ok {
			if got := enabled == 1; got != wantValue {
				t.Errorf("Kanban step %q cancel trigger = %t, want %t", name, got, wantValue)
			}
			seen++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Kanban flags: %v", err)
	}
	if seen != len(want) {
		t.Fatalf("observed %d Kanban steps with cancel flags, want %d", seen, len(want))
	}
}

func TestEnsureOfficeDefaultWorkflow_TriggersJSONShape(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()

	id, err := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ensure office-default: %v", err)
	}
	steps := loadStepsForWorkflow(t, repo, id)
	work := findStepByNameLocal(steps, "Work")
	if work == nil {
		t.Fatalf("Work step not found")
	}

	// on_enter: auto_start_agent
	if len(work.Events.OnEnter) != 1 {
		t.Fatalf("Work.on_enter len = %d, want 1", len(work.Events.OnEnter))
	}
	if work.Events.OnEnter[0].Type != wfmodels.OnEnterAutoStartAgent {
		t.Errorf("Work.on_enter[0] = %q, want auto_start_agent", work.Events.OnEnter[0].Type)
	}

	// on_comment: queue_run target=primary
	if len(work.Events.OnComment) != 1 {
		t.Fatalf("Work.on_comment len = %d, want 1", len(work.Events.OnComment))
	}
	if work.Events.OnComment[0].Type != wfmodels.GenericActionQueueRun {
		t.Errorf("Work.on_comment[0] type = %q, want queue_run", work.Events.OnComment[0].Type)
	}
	if got, _ := work.Events.OnComment[0].Config["target"].(string); got != "primary" {
		t.Errorf("Work.on_comment[0] target = %q, want primary", got)
	}
	if got, _ := work.Events.OnComment[0].Config["reason"].(string); got != "task_comment" {
		t.Errorf("Work.on_comment[0] reason = %q, want task_comment", got)
	}

	// on_agent_error: queue_run target=workspace.ceo_agent
	if len(work.Events.OnAgentError) != 1 {
		t.Fatalf("Work.on_agent_error len = %d, want 1", len(work.Events.OnAgentError))
	}
	if got, _ := work.Events.OnAgentError[0].Config["target"].(string); got != "workspace.ceo_agent" {
		t.Errorf("Work.on_agent_error[0] target = %q, want workspace.ceo_agent", got)
	}
}

func TestEnsureOfficeDefaultWorkflow_ReviewClearsAndFansOut(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	id, _ := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-1")

	steps := loadStepsForWorkflow(t, repo, id)
	review := findStepByNameLocal(steps, "Review")
	if review == nil {
		t.Fatalf("Review step not found")
	}
	if len(review.Events.OnEnter) != 3 {
		t.Fatalf("Review.on_enter len = %d, want 3 (clear_decisions + ensure_participant_seat + queue_run_for_each_participant)", len(review.Events.OnEnter))
	}
	if review.Events.OnEnter[0].Type != wfmodels.OnEnterClearDecisions {
		t.Errorf("Review.on_enter[0] = %q, want clear_decisions", review.Events.OnEnter[0].Type)
	}
	if review.Events.OnEnter[1].Type != wfmodels.OnEnterEnsureParticipantSeat {
		t.Errorf("Review.on_enter[1] = %q, want ensure_participant_seat", review.Events.OnEnter[1].Type)
	}
	if seatRole, _ := review.Events.OnEnter[1].Config["role"].(string); seatRole != "reviewer" {
		t.Errorf("Review.on_enter[1] role = %q, want reviewer", seatRole)
	}
	if review.Events.OnEnter[2].Type != wfmodels.OnEnterQueueRunForEachParticipant {
		t.Errorf("Review.on_enter[2] = %q, want queue_run_for_each_participant", review.Events.OnEnter[2].Type)
	}
	role, _ := review.Events.OnEnter[2].Config["role"].(string)
	if role != "reviewer" {
		t.Errorf("Review.on_enter[2] role = %q, want reviewer", role)
	}

	// on_turn_complete must have two guarded transitions.
	if len(review.Events.OnTurnComplete) != 2 {
		t.Fatalf("Review.on_turn_complete len = %d, want 2", len(review.Events.OnTurnComplete))
	}
}

func TestEnsureOfficeDefaultWorkflow_Idempotent(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	first, err := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	second, err := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if first != second {
		t.Errorf("expected idempotent id, got %q vs %q", first, second)
	}
	assertBuiltinWorkflowHidden(t, repo, first)
	steps := loadStepsForWorkflow(t, repo, first)
	if len(steps) != 5 {
		t.Errorf("expected 5 steps after idempotent ensure, got %d", len(steps))
	}
}

// TestEnsureRoutineWorkflow_AutoCompletingShape verifies the routine
// system workflow has the two-step shape from spec PR 3:
// "In Progress" (start, on_enter:auto_start_agent, on_turn_complete:
// move_to_step→done) and "Done". Idempotent on re-call.
func TestEnsureRoutineWorkflow_AutoCompletingShape(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()

	first, err := repo.EnsureRoutineWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ensure routine: %v", err)
	}
	second, err := repo.EnsureRoutineWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ensure routine (idempotent): %v", err)
	}
	if first != second {
		t.Errorf("expected idempotent id, got %q vs %q", first, second)
	}
	assertBuiltinWorkflowHidden(t, repo, first)

	steps := loadStepsForWorkflow(t, repo, first)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	in := findStepByNameLocal(steps, "In Progress")
	if in == nil {
		t.Fatal("missing In Progress step")
	}
	if len(in.Events.OnEnter) != 1 || in.Events.OnEnter[0].Type != "auto_start_agent" {
		t.Errorf("In Progress.on_enter = %+v, want [auto_start_agent]", in.Events.OnEnter)
	}
	if len(in.Events.OnTurnComplete) != 1 || in.Events.OnTurnComplete[0].Type != "move_to_step" {
		t.Errorf("In Progress.on_turn_complete = %+v, want [move_to_step]", in.Events.OnTurnComplete)
	}
	if findStepByNameLocal(steps, "Done") == nil {
		t.Fatal("missing Done step")
	}
}

func TestEnsureRoutineWorkflow_HealsExistingWorkflowVisibility(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()

	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES (
			'stale-routine', 'ws-1', 'Routine', 'routine', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("insert stale routine workflow: %v", err)
	}

	id, err := repo.EnsureRoutineWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ensure routine: %v", err)
	}
	if id != "stale-routine" {
		t.Fatalf("EnsureRoutineWorkflow() id = %q, want stale-routine", id)
	}
	assertBuiltinWorkflowHidden(t, repo, id)
}

func TestEnsureRoutineWorkflow_KeepsUserWorkflowVisible(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()

	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES (
			'user-routine', 'ws-1', 'My Routine', 'routine', 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("insert user routine workflow: %v", err)
	}

	id, err := repo.EnsureRoutineWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ensure routine: %v", err)
	}
	if id == "user-routine" {
		t.Fatal("EnsureRoutineWorkflow() reused a user workflow")
	}

	var hidden int
	if err := repo.db.QueryRowContext(ctx, `SELECT hidden FROM workflows WHERE id = 'user-routine'`).Scan(&hidden); err != nil {
		t.Fatalf("query user routine visibility: %v", err)
	}
	if hidden != 0 {
		t.Errorf("user routine hidden = %d, want 0", hidden)
	}
}

func TestRepositoryInitialization_HealsBuiltinWorkflowVisibility(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	for _, workflow := range []struct {
		id         string
		templateID string
	}{
		{id: "stale-office", templateID: "office-default"},
		{id: "stale-routine", templateID: "routine"},
	} {
		_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			INSERT INTO workflows (
				id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
			) VALUES (?, 'ws-1', 'System workflow', ?, 1, 0, ?, ?)
		`), workflow.id, workflow.templateID, legacyTime, legacyTime)
		if err != nil {
			t.Fatalf("insert stale %s workflow: %v", workflow.templateID, err)
		}
	}

	if err := repo.initSchema(); err != nil {
		t.Fatalf("reinitialize repository: %v", err)
	}
	assertBuiltinWorkflowHidden(t, repo, "stale-office")
	assertBuiltinWorkflowHidden(t, repo, "stale-routine")

	for _, workflowID := range []string{"stale-office", "stale-routine"} {
		var updatedAt time.Time
		if err := repo.db.QueryRowContext(ctx, `SELECT updated_at FROM workflows WHERE id = ?`, workflowID).Scan(&updatedAt); err != nil {
			t.Fatalf("query workflow updated_at: %v", err)
		}
		if !updatedAt.After(legacyTime) {
			t.Errorf("workflow %q updated_at = %v, want after %v", workflowID, updatedAt, legacyTime)
		}
	}
}

func TestHealBuiltinWorkflowStepFlags_HealsStaleOfficeWorkStep(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('stale-office-heal', 'ws-1', 'Office', 'office-default', 1, 1, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale office workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('stale-office-heal-work', 'stale-office-heal', 'Work', 1, 'work', '{}', 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale work step: %v", err)
	}

	if err := repo.healBuiltinWorkflowStepFlags(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepFlags: %v", err)
	}

	steps := loadStepsForWorkflow(t, repo, "stale-office-heal")
	work := findStepByNameLocal(steps, "Work")
	if work == nil {
		t.Fatal("missing Work step")
	}
	if !work.AutoAdvanceRequiresSignal {
		t.Error("healBuiltinWorkflowStepFlags() left Work.AutoAdvanceRequiresSignal = false, want true")
	}

	var updatedAt time.Time
	if err := repo.db.QueryRowContext(ctx, `SELECT updated_at FROM workflow_steps WHERE id = 'stale-office-heal-work'`).Scan(&updatedAt); err != nil {
		t.Fatalf("query step updated_at: %v", err)
	}
	if !updatedAt.After(legacyTime) {
		t.Errorf("Work step updated_at = %v, want after %v", updatedAt, legacyTime)
	}
}

// TestRepositoryInitialization_HealsBuiltinWorkflowStepFlags verifies that
// initSchema keeps the boot registration for the reconciler. Direct tests
// above would not catch removing the step from the initialization sequence.
func TestRepositoryInitialization_HealsBuiltinWorkflowStepFlags(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('stale-office-boot', 'ws-1', 'Office', 'office-default', 1, 1, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale boot workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('stale-office-boot-work', 'stale-office-boot', 'Work', 1, 'work', '{}', 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert stale boot work step: %v", err)
	}

	if err := repo.initSchema(); err != nil {
		t.Fatalf("reinitialize repository: %v", err)
	}

	steps := loadStepsForWorkflow(t, repo, "stale-office-boot")
	work := findStepByNameLocal(steps, "Work")
	if work == nil {
		t.Fatal("missing Work step")
	}
	if !work.AutoAdvanceRequiresSignal {
		t.Error("initSchema left Work.AutoAdvanceRequiresSignal = false, want true")
	}
}

func TestHealBuiltinWorkflowStepFlags_KeepsUserWorkflowStepUntouched(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	legacyTime := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflows (
			id, workspace_id, name, workflow_template_id, is_system, hidden, created_at, updated_at
		) VALUES ('user-office', 'ws-1', 'My Office', 'office-default', 0, 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user office workflow: %v", err)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, stage_type, events, auto_advance_requires_signal, created_at, updated_at
		) VALUES ('user-office-work', 'user-office', 'Work', 1, 'work', '{}', 0, ?, ?)
	`), legacyTime, legacyTime)
	if err != nil {
		t.Fatalf("insert user work step: %v", err)
	}

	if err := repo.healBuiltinWorkflowStepFlags(); err != nil {
		t.Fatalf("healBuiltinWorkflowStepFlags: %v", err)
	}

	steps := loadStepsForWorkflow(t, repo, "user-office")
	work := findStepByNameLocal(steps, "Work")
	if work == nil {
		t.Fatal("missing Work step")
	}
	if work.AutoAdvanceRequiresSignal {
		t.Error("healBuiltinWorkflowStepFlags() touched a user-customised (is_system=0) workflow's step")
	}
}

// TestRoutineWorkflowTasks_FlaggedSystem verifies tasks created in the
// routine workflow are excluded from ListTasksByWorkspace by default
// (template_id="routine" lives in SystemWorkflowTemplateIDs).
func TestRoutineWorkflowTasks_FlaggedSystem(t *testing.T) {
	for _, want := range SystemWorkflowTemplateIDs {
		if want == "routine" {
			return
		}
	}
	t.Errorf("SystemWorkflowTemplateIDs missing 'routine': %v", SystemWorkflowTemplateIDs)
}

func findStepByNameLocal(steps []*wfmodels.WorkflowStep, name string) *wfmodels.WorkflowStep {
	for _, s := range steps {
		if s.Name == name {
			return s
		}
	}
	return nil
}
