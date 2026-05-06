package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
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
	repo, err := NewWithDB(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	// The Phase 6 ensure helpers write into workflow_steps, whose schema
	// is initialised by the workflow repository. Build it on the same DB
	// so tests exercise the production migration order.
	if _, err := workflowrepo.NewWithDB(sqlxDB, sqlxDB); err != nil {
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
		SELECT id, name, position, stage_type, events
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
		if err := rows.Scan(&step.ID, &step.Name, &step.Position, &stage, &events); err != nil {
			t.Fatalf("scan step: %v", err)
		}
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
	if len(review.Events.OnEnter) != 2 {
		t.Fatalf("Review.on_enter len = %d, want 2 (clear_decisions + queue_run_for_each_participant)", len(review.Events.OnEnter))
	}
	if review.Events.OnEnter[0].Type != wfmodels.OnEnterClearDecisions {
		t.Errorf("Review.on_enter[0] = %q, want clear_decisions", review.Events.OnEnter[0].Type)
	}
	if review.Events.OnEnter[1].Type != wfmodels.OnEnterQueueRunForEachParticipant {
		t.Errorf("Review.on_enter[1] = %q, want queue_run_for_each_participant", review.Events.OnEnter[1].Type)
	}
	role, _ := review.Events.OnEnter[1].Config["role"].(string)
	if role != "reviewer" {
		t.Errorf("Review.on_enter[1] role = %q, want reviewer", role)
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
