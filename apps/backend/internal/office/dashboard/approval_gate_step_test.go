package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

// insertTestTaskAtNonTerminalStep creates a two-step workflow (stepID at
// position 0, named stepName; a later step at position 1) and a task
// pointing at stepID, so IsTaskWorkflowStepTerminal reports false for it
// regardless of the step's own name.
func insertTestTaskAtNonTerminalStep(t *testing.T, db sqlxExecutor, id, wsID, title, state, stepName string) {
	t.Helper()
	workflowID := "wf-nonterm-" + id
	stepID := "step-" + id
	if _, err := db.Exec(`
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, 'Test Workflow', datetime('now'), datetime('now'))
	`, workflowID, wsID); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES (?, ?, ?, 0, datetime('now'), datetime('now'))
	`, stepID, workflowID, stepName); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES (?, ?, 'Next', 1, datetime('now'), datetime('now'))
	`, stepID+"-next", workflowID); err != nil {
		t.Fatalf("seed next step: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, state, priority, identifier, workflow_step_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'medium', ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, title, state, id, stepID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

// TestUpdateTaskStatus_AgentDoneAtWork_RedirectsToReview is test-matrix
// case (1): an agent-authored "done" write from a non-terminal ("Work")
// step is redirected to in_review regardless of approvers — the task
// never had any.
func TestUpdateTaskStatus_AgentDoneAtWork_RedirectsToReview(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "wk1", "ws-g", "WK", "in_progress", "Work")

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "wk1",
		NewStatus: "done",
	})
	var pending *dashboard.ApprovalsPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("err = %v, want *ApprovalsPendingError", err)
	}
	if len(pending.Pending) != 0 {
		t.Errorf("pending = %v, want none: the redirect is on step position, not approvers", pending.Pending)
	}
	if state := readTaskState(t, deps, "wk1"); state != "REVIEW" {
		t.Errorf("state = %q, want REVIEW", state)
	}
}

// TestUpdateTaskStatus_AgentDoneAtReview_NoApproverSeats_RedirectsToReview
// is test-matrix case (2) and the regression test for the live bug: five
// Office tasks were observed reading COMPLETED while sitting on the
// Review step. The old gate only checked for pending approvers, and a
// task at Review has never entered the Approval step that creates
// approver seats — so pendingApprovers always returned empty and the
// gate never fired for exactly this case. This must fail against a gate
// that only checks approvers, and pass once it also checks step position.
func TestUpdateTaskStatus_AgentDoneAtReview_NoApproverSeats_RedirectsToReview(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "rv1", "ws-g", "RV", "in_review", "Review")

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "rv1",
		NewStatus: "done",
	})
	var pending *dashboard.ApprovalsPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("err = %v, want *ApprovalsPendingError", err)
	}
	if state := readTaskState(t, deps, "rv1"); state != "REVIEW" {
		t.Errorf("state = %q, want REVIEW (task must never read COMPLETED while on the Review step)", state)
	}
}

// erroringTerminalStepRepo wraps a real *sqlite.Repository, replacing
// IsTaskWorkflowStepTerminal with one that always fails, to test the
// gate's behavior on a transient lookup error.
type erroringTerminalStepRepo struct {
	*sqlite.Repository
	err error
}

func (r *erroringTerminalStepRepo) IsTaskWorkflowStepTerminal(context.Context, string) (bool, error) {
	return false, r.err
}

// TestUpdateTaskStatus_TerminalStepLookupError_AbortsWithoutPersistingRedirect
// is the MAJOR-2 regression test (review round 2): a transient
// IsTaskWorkflowStepTerminal error must not be folded into "not terminal"
// and persisted as a REVIEW redirect, since nothing then re-evaluates the
// gate and the task is stranded reporting in_review forever with no
// approver to clear it. A lookup error must abort the whole status update
// (a plain, non-ApprovalsPendingError error) and leave the prior state
// intact — still refusing the "done" write, but without a bogus redirect.
func TestUpdateTaskStatus_TerminalStepLookupError_AbortsWithoutPersistingRedirect(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "lookup-err", "ws-g", "LE", "in_progress", 1)

	wrapped := &erroringTerminalStepRepo{Repository: deps.repo, err: errors.New("database is locked")}
	log := logger.Default()
	svc := dashboard.NewDashboardService(wrapped, log, shared.NewActivityLogger(deps.repo, log), deps.agents, &stubCostChecker{})

	err := svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "lookup-err",
		NewStatus: "done",
	})

	var pending *dashboard.ApprovalsPendingError
	if errors.As(err, &pending) {
		t.Fatalf("err = %v, want a plain error: a lookup failure is not a confirmed redirect", err)
	}
	if err == nil {
		t.Fatal("err = nil, want an error: a lookup failure must not silently let the task complete")
	}
	if state := readTaskState(t, deps, "lookup-err"); state != "in_progress" {
		t.Errorf("state = %q, want unchanged in_progress (no redirect and no completion persisted)", state)
	}
}
