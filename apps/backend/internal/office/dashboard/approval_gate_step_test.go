package dashboard_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestUpdateTaskStatus_RequiredRoutineAssignmentsBlockOfficeCompletion(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()
	taskRepo, err := tasksqlite.NewWithDB(deps.db, deps.db, nil)
	if err != nil {
		t.Fatalf("task repo: %v", err)
	}
	deps.svc.SetTaskStateWriter(taskRepo)
	metadata, err := json.Marshal(map[string]int{taskmodels.MetaKeyRequiredSubtaskCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deps.db.Exec(`INSERT INTO workflows (id, workspace_id, name, created_at, updated_at) VALUES ('wf-required', 'ws-g', 'Routine', datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := deps.db.Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at) VALUES ('step-required', 'wf-required', 'Done', 0, datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("insert terminal step: %v", err)
	}
	if _, err := deps.db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, state, priority, identifier, workflow_id, workflow_step_id, metadata, created_at, updated_at)
		VALUES ('required-office-task', 'ws-g', 'Routine task', 'IN_PROGRESS', 'medium', 'ROT-1', 'wf-required', 'step-required', ?, datetime('now'), datetime('now'))
	`, string(metadata)); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	err = deps.svc.UpdateTaskStatus(ctx, dashboard.TaskStatusUpdateRequest{
		TaskID:    "required-office-task",
		NewStatus: "done",
	})
	if err == nil || !strings.Contains(err.Error(), "required subtask assignments") {
		t.Fatalf("UpdateTaskStatus error = %v, want assignment guard error", err)
	}
	if state := readTaskState(t, deps, "required-office-task"); state != "IN_PROGRESS" {
		t.Fatalf("state = %q, want IN_PROGRESS", state)
	}
}

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
	if got := pending.ReasonCode(); got != dashboard.ApprovalGateReasonWorkflowStep {
		t.Errorf("reason = %q, want %q", got, dashboard.ApprovalGateReasonWorkflowStep)
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

// insertChannelShapedTask inserts a task exactly as
// office/repository/sqlite/channels.go CreateChannelTask does: no
// workflow_id, no workflow_step_id. Channel tasks (the long-lived
// Telegram/Slack/email assistant tasks) are the production shape that
// exercises the "no workflow step at all" branch of the approval gate.
func insertChannelShapedTask(t *testing.T, db sqlxExecutor, id, wsID, title string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, state, created_at, updated_at)
		VALUES (?, ?, ?, 'IN_PROGRESS', datetime('now'), datetime('now'))
	`, id, wsID, title); err != nil {
		t.Fatalf("insert channel-shaped task: %v", err)
	}
}

// TestUpdateTaskStatus_ChannelTaskNoWorkflowStep_ReachesDone is the
// regression test for Build round 5, Finding 1: a task with no
// workflow_step_id at all (the channel-task shape) must still be able to
// reach done. IsTaskWorkflowStepTerminal reporting "not terminal" for a
// step that does not exist is not the same fact as a task sitting on a
// genuine non-terminal step, and the gate must not conflate the two.
func TestUpdateTaskStatus_ChannelTaskNoWorkflowStep_ReachesDone(t *testing.T) {
	deps := newTestDeps(t)
	insertChannelShapedTask(t, deps.db, "chan1", "ws-g", "Channel task")

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "chan1",
		NewStatus: "done",
	})
	if err != nil {
		t.Fatalf("err = %v, want nil: a task with no workflow step must fall through to the approver check", err)
	}
	if state := readTaskState(t, deps, "chan1"); state != "COMPLETED" {
		t.Errorf("state = %q, want COMPLETED", state)
	}
}

// erroringTerminalStepRepo wraps a real *sqlite.Repository, replacing
// IsTaskWorkflowStepTerminal with one that always fails, to test the
// gate's behavior on a transient lookup error.
type erroringTerminalStepRepo struct {
	*sqlite.Repository
	err error
}

func (r *erroringTerminalStepRepo) IsTaskWorkflowStepTerminal(context.Context, string) (bool, bool, error) {
	return false, false, r.err
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
	svc.SetTaskStateWriter(directTaskStateWriter{db: deps.db})

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

// The automatic-completion tests below pin the #4889 state-plus-step
// compare-and-set. A stale automatic write must fail closed before any
// status, activity, task.updated, or reactivity side effect runs.

func TestUpdateTaskStatus_AutomaticCompletionRejectsStateMismatch(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "auto-state", "ws-auto", "Auto state", "IN_PROGRESS", 0)
	pub := &recordingTaskLifecyclePublisher{}
	deps.svc.SetTaskLifecyclePublisher(pub)
	reactivity := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	deps.svc.SetReactivityApplier(reactivity)

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:                 "auto-state",
		NewStatus:              "done",
		SuppressStatusActivity: true,
		ExpectedCurrentState:   "TODO",
		ExpectedWorkflowStepID: "step-auto-state",
	})
	if !errors.Is(err, dashboard.ErrAutomaticCompletionStale) {
		t.Fatalf("err = %v, want ErrAutomaticCompletionStale", err)
	}
	if state := readTaskState(t, deps, "auto-state"); state != "IN_PROGRESS" {
		t.Fatalf("state = %q, want IN_PROGRESS unchanged", state)
	}
	if len(pub.published) != 0 {
		t.Fatalf("task.updated published = %v, want none for a stale automatic write", pub.published)
	}
	if len(reactivity.calls) != 0 {
		t.Fatalf("reactivity calls = %d, want none for a stale automatic write", len(reactivity.calls))
	}
	if hasStatusChangeActivity(t, deps, "auto-state", "ws-auto") {
		t.Fatal("stale automatic write logged a task_status_changed activity")
	}
}

// TestUpdateTaskStatus_AutomaticCompletionRejectsGateStepMismatch pins the
// approval-step edge: the gate may observe a different step than the caller's
// snapshot, and the automatic write must fail closed instead of using that
// newer step.
func TestUpdateTaskStatus_AutomaticCompletionRejectsGateStepMismatch(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "auto-step", "ws-auto", "Auto step", "IN_PROGRESS", 0)
	pub := &recordingTaskLifecyclePublisher{}
	deps.svc.SetTaskLifecyclePublisher(pub)
	reactivity := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	deps.svc.SetReactivityApplier(reactivity)

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:                 "auto-step",
		NewStatus:              "done",
		SuppressStatusActivity: true,
		ExpectedCurrentState:   "IN_PROGRESS",
		ExpectedWorkflowStepID: "step-observed-before-the-gate",
	})
	if !errors.Is(err, dashboard.ErrAutomaticCompletionStale) {
		t.Fatalf("err = %v, want ErrAutomaticCompletionStale", err)
	}
	if state := readTaskState(t, deps, "auto-step"); state != "IN_PROGRESS" {
		t.Fatalf("state = %q, want IN_PROGRESS unchanged", state)
	}
	if len(pub.published) != 0 {
		t.Fatalf("task.updated published = %v, want none when the gate step differs", pub.published)
	}
	if len(reactivity.calls) != 0 {
		t.Fatalf("reactivity calls = %d, want none when the gate step differs", len(reactivity.calls))
	}
}

func TestUpdateTaskStatus_AutomaticCompletionAppliesWhenStateAndStepMatch(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "auto-match", "ws-auto", "Auto match", "TODO", 0)
	pub := &recordingTaskLifecyclePublisher{}
	deps.svc.SetTaskLifecyclePublisher(pub)

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:                 "auto-match",
		NewStatus:              "done",
		SuppressStatusActivity: true,
		ExpectedCurrentState:   "TODO",
		ExpectedWorkflowStepID: "step-auto-match",
	})
	if err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if state := readTaskState(t, deps, "auto-match"); state != "COMPLETED" {
		t.Fatalf("state = %q, want COMPLETED", state)
	}
	if len(pub.published) != 1 || pub.published[0] != "auto-match" {
		t.Fatalf("task.updated published = %v, want exactly one for auto-match", pub.published)
	}
}
