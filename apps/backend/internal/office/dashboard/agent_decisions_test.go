package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// insertTestTaskNoStep inserts a task row with no workflow_step_id bound
// (the schema default is ” — see tasks.workflow_step_id NOT NULL DEFAULT
// ”), exercising the AC-7/AC-55(1) precondition RecordAgentDecision must
// reject before any role resolution or write is attempted.
func insertTestTaskNoStep(t *testing.T, deps *testDeps, id, wsID, title string) {
	t.Helper()
	_, err := deps.db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, state, priority, identifier, created_at, updated_at)
		VALUES (?, ?, ?, 'in_review', 'medium', ?, datetime('now'), datetime('now'))
	`, id, wsID, title, id)
	if err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

// TestRecordAgentDecision_ApproverHappyPath is AC-1/2/6/7/8: an agent
// occupying an approver seat can record a decision, and the result mirrors
// the engine's write plus the AC-64 default no-transition/no-guards shape
// this test fixture's session-less dispatcher always produces (per
// engine_test_helpers_test.go: fakeSessionResolver reports no active
// session, so RecordParticipantDecision's re-evaluation subpath — and thus
// any transition — never runs).
func TestRecordAgentDecision_ApproverHappyPath(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ad1", "ws-d", "AD", "in_review", 2)
	mustAddParticipant(t, deps, "ad1", "agent-1", models.ParticipantRoleApprover)

	res, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad1",
		AgentProfileID: "agent-1",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	if res.Role != models.ParticipantRoleApprover {
		t.Errorf("Role = %q, want %q", res.Role, models.ParticipantRoleApprover)
	}
	if res.Decision != engine.DecisionApproved {
		t.Errorf("Decision = %q, want %q", res.Decision, engine.DecisionApproved)
	}
	if res.StepID != "step-ad1" {
		t.Errorf("StepID = %q, want step-ad1", res.StepID)
	}
	if res.DecisionID == "" {
		t.Error("DecisionID is empty")
	}
	if res.TransitionApplied {
		t.Error("TransitionApplied = true, want false (no active session in this fixture)")
	}
	if len(res.Guards) != 0 {
		t.Errorf("Guards = %+v, want empty", res.Guards)
	}

	rows, err := deps.svc.ListTaskDecisions(context.Background(), "ad1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("decisions = %d, want 1", len(rows))
	}
	if rows[0].DeciderType != models.DeciderTypeAgent || rows[0].DeciderID != "agent-1" {
		t.Errorf("decider = %s/%s, want agent/agent-1", rows[0].DeciderType, rows[0].DeciderID)
	}
}

// TestRecordAgentDecision_ApproverWinsOverReviewer is AC-4: when an agent
// occupies both seats at the same step, the decision is recorded under the
// approver role.
func TestRecordAgentDecision_ApproverWinsOverReviewer(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ad2", "ws-d", "AD2", "in_review", 2)
	mustAddParticipant(t, deps, "ad2", "agent-X", models.ParticipantRoleReviewer)
	mustAddParticipant(t, deps, "ad2", "agent-X", models.ParticipantRoleApprover)

	res, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad2",
		AgentProfileID: "agent-X",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	if res.Role != models.ParticipantRoleApprover {
		t.Errorf("Role = %q, want %q (approver-wins)", res.Role, models.ParticipantRoleApprover)
	}
}

// TestRecordAgentDecision_ReviewerSeat is AC-2/3: an agent occupying only
// a reviewer seat records under the reviewer role.
func TestRecordAgentDecision_ReviewerSeat(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ad3", "ws-d", "AD3", "in_review", 2)
	mustAddParticipant(t, deps, "ad3", "agent-2", models.ParticipantRoleReviewer)

	res, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad3",
		AgentProfileID: "agent-2",
		Decision:       engine.DecisionRejected,
		Reason:         "needs work",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	if res.Role != models.ParticipantRoleReviewer {
		t.Errorf("Role = %q, want %q", res.Role, models.ParticipantRoleReviewer)
	}
}

// TestRecordAgentDecision_ForbiddenWhenNotParticipant is AC-3: an agent
// occupying neither seat is rejected with shared.ErrForbidden, and no row
// is written.
func TestRecordAgentDecision_ForbiddenWhenNotParticipant(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ad4", "ws-d", "AD4", "in_review", 2)

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad4",
		AgentProfileID: "agent-zzz",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}

	rows, err := deps.svc.ListTaskDecisions(context.Background(), "ad4")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("decisions = %d, want 0", len(rows))
	}
}

// TestRecordAgentDecision_NoWorkflowStepBoundRejected is AC-7/AC-55(1):
// a task with no workflow_step_id bound is rejected before any role
// resolution is attempted.
func TestRecordAgentDecision_NoWorkflowStepBoundRejected(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskNoStep(t, deps, "ad5", "ws-d", "AD5")

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad5",
		AgentProfileID: "agent-1",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
	})
	if err == nil {
		t.Fatal("expected error for task with no workflow step bound")
	}
}

// TestRecordAgentDecision_InvalidVerdictRejected is AC-5/AC-55(3): a
// decision must resolve to a validated seat before its verdict is checked,
// but an unrecognized verdict is still rejected and no row is written.
func TestRecordAgentDecision_InvalidVerdictRejected(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ad6", "ws-d", "AD6", "in_review", 2)
	mustAddParticipant(t, deps, "ad6", "agent-1", models.ParticipantRoleApprover)

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad6",
		AgentProfileID: "agent-1",
		Decision:       "bogus",
		Reason:         "lgtm",
	})
	if err == nil {
		t.Fatal("expected error for invalid decision verdict")
	}

	rows, err := deps.svc.ListTaskDecisions(context.Background(), "ad6")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("decisions = %d, want 0", len(rows))
	}
}

// TestRecordAgentDecision_EmptyReasonRejected is AC-6/AC-55(4): a blank
// reason is rejected regardless of verdict validity.
func TestRecordAgentDecision_EmptyReasonRejected(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ad7", "ws-d", "AD7", "in_review", 2)
	mustAddParticipant(t, deps, "ad7", "agent-1", models.ParticipantRoleApprover)

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad7",
		AgentProfileID: "agent-1",
		Decision:       engine.DecisionApproved,
		Reason:         "   ",
	})
	if err == nil {
		t.Fatal("expected error for blank reason")
	}
}

// TestRecordAgentDecision_RejectsWhenEngineDispatcherNotWired is AC-57c's
// agent-path counterpart to TestApproveTask_RejectsWhenEngineDispatcherNotWired:
// with no engine dispatcher wired, the call is rejected and no row is
// written — never a fallback to a second, office-side write.
func TestRecordAgentDecision_RejectsWhenEngineDispatcherNotWired(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetWorkflowEngineDispatcher(nil)
	insertTestTask(t, deps.db, "ad8", "ws-d", "AD8", "in_review", 2)
	mustAddParticipant(t, deps, "ad8", "agent-1", models.ParticipantRoleApprover)

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad8",
		AgentProfileID: "agent-1",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
	})
	if err == nil {
		t.Fatal("expected error when engine dispatcher is not wired")
	}
}

// TestRecordAgentDecision_BindsToSuppliedSessionOverActiveSibling is the
// end-to-end proof (through the real engine, not a fake) that
// RecordAgentDecisionInput.SessionID actually reaches
// RecordParticipantDecision's re-evaluation subpath: once a task can carry
// more than one concurrent active session (WO-72), a task-scoped
// active-session lookup (started_at DESC) could return an unrelated sibling
// instead of the session that made this decision. deps.svc.RecordAgentDecision
// is only the entry point — twoSessionResolver models the two-session
// scenario and dashboardTransitionStore.loadStateSessionIDs captures
// which session id the engine's LoadState was actually called with, so this
// asserts the binding itself rather than just that the write succeeded
// (which it does either way).
func TestRecordAgentDecision_BindsToSuppliedSessionOverActiveSibling(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "ad9", "ws-d", "AD9", "in_review", 2)
	mustAddParticipant(t, deps, "ad9", "agent-1", models.ParticipantRoleApprover)

	store := newDashboardTransitionStore(deps.db)
	// Leave workflowID blank (unlike the "wf-test" default): a non-empty
	// WorkflowID would route ResolveParticipantRole's slate construction
	// through the workflow-scoped participant query, which joins against a
	// workflow_steps row this test does not seed. Every other
	// RecordAgentDecision test resolves participants task-scoped (no
	// workflow_steps row involved either), and that's the behavior this
	// test needs too — only the session binding is under test here.
	store.workflowID = ""
	activeSibling := &taskmodels.TaskSession{
		ID: "sess-sibling", TaskID: "ad9", State: taskmodels.TaskSessionStateRunning,
	}
	reviewerSession := &taskmodels.TaskSession{
		ID: "sess-reviewer", TaskID: "ad9", State: taskmodels.TaskSessionStateRunning,
	}
	resolver := twoSessionResolver{taskID: "ad9", active: activeSibling, other: reviewerSession}
	deps.svc.SetWorkflowEngineDispatcher(
		newTestEngineDispatcherWithResolver(deps.wfRepo, logger.Default(), store, resolver),
	)

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "ad9",
		AgentProfileID: "agent-1",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
		SessionID:      "sess-reviewer",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	// loadStateSessionIDs also picks up two calls that are not the write
	// path under test: ResolveParticipantRole's own LoadState(ctx, taskID,
	// "") probe (blank sessionID, runs first) and the dashboard's
	// post-write guards-snapshot diagnostic, which deliberately reads the
	// task-scoped active session ("sess-sibling") instead — see
	// resolveLatestSessionID's doc comment. The first *non-blank* session
	// id is RecordParticipantDecision's re-evaluation subpath, which is
	// what RecordDecision bound.
	var boundSessionID string
	for _, id := range store.loadStateSessionIDs {
		if id != "" {
			boundSessionID = id
			break
		}
	}
	if boundSessionID != "sess-reviewer" {
		t.Errorf("bound session id = %q (all LoadState calls: %v), want sess-reviewer (the deciding agent's own session, not the active sibling sess-sibling)",
			boundSessionID, store.loadStateSessionIDs)
	}
}
