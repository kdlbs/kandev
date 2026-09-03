package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	officeenginedispatcher "github.com/kandev/kandev/internal/office/engine_dispatcher"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// spyDecisionDispatcher satisfies shared.WorkflowEngineDispatcher plus the
// package-local decisionRecordingDispatcher/roleResolvingDispatcher/
// quorumEvaluatingDispatcher capabilities RecordAgentDecision reaches via
// type assertion (all exported methods, so a type from this external test
// package can still satisfy those unexported interfaces structurally).
// Unlike newTestEngineDispatcher's real engine, this spy captures the exact
// RecordDecisionInput it was called with, so a test can assert whether
// SessionID was forwarded without needing a live session row to observe an
// engine-side behavior difference.
type spyDecisionDispatcher struct {
	role          string
	participantID string
	lastInput     officeenginedispatcher.RecordDecisionInput
}

func (s *spyDecisionDispatcher) HandleTrigger(
	_ context.Context, _ string, _ engine.Trigger, _ any, _ string,
) error {
	return nil
}

func (s *spyDecisionDispatcher) ResolveParticipantRole(
	_ context.Context, _, _, _ string,
) (string, string, error) {
	return s.role, s.participantID, nil
}

func (s *spyDecisionDispatcher) RecordDecision(
	_ context.Context, in officeenginedispatcher.RecordDecisionInput,
) (officeenginedispatcher.RecordDecisionResult, error) {
	s.lastInput = in
	return officeenginedispatcher.RecordDecisionResult{DecisionID: "decision-spy"}, nil
}

func (s *spyDecisionDispatcher) EvaluateStepQuorum(
	_ context.Context, _ string,
) (engine.QuorumSnapshot, error) {
	return engine.QuorumSnapshot{}, nil
}

// TestRecordAgentDecision_SessionIDNotForwardedWhenFlagOff is the default
// (flag-off) case: even though the caller supplies a SessionID, the
// dispatcher never sees it, so RecordDecision falls back to its existing
// resolveActiveSessionID path — the pre-office-session-identity behavior.
func TestRecordAgentDecision_SessionIDNotForwardedWhenFlagOff(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "sid1", "ws-d", "SID1", "in_review", 2)
	mustAddParticipant(t, deps, "sid1", "agent-1", models.ParticipantRoleApprover)
	spy := &spyDecisionDispatcher{role: models.ParticipantRoleApprover, participantID: "participant-1"}
	deps.svc.SetWorkflowEngineDispatcher(spy)

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "sid1",
		AgentProfileID: "agent-1",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
		SessionID:      "sess-caller",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	if spy.lastInput.SessionID != "" {
		t.Errorf("SessionID = %q, want blank (flag is off)", spy.lastInput.SessionID)
	}
}

// TestRecordAgentDecision_SessionIDForwardedWhenFlagOn proves that once
// features.officeSessionIdentity is enabled, RecordAgentDecision forwards
// the caller's own SessionID into RecordDecisionInput, so RecordDecision
// re-evaluates against the decider's own session instead of the task's
// most-recently-started ("active") one.
func TestRecordAgentDecision_SessionIDForwardedWhenFlagOn(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "sid2", "ws-d", "SID2", "in_review", 2)
	mustAddParticipant(t, deps, "sid2", "agent-1", models.ParticipantRoleApprover)
	spy := &spyDecisionDispatcher{role: models.ParticipantRoleApprover, participantID: "participant-1"}
	deps.svc.SetWorkflowEngineDispatcher(spy)
	deps.svc.SetOfficeSessionIdentity(true)

	_, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "sid2",
		AgentProfileID: "agent-1",
		Decision:       engine.DecisionApproved,
		Reason:         "lgtm",
		SessionID:      "sess-caller",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	if spy.lastInput.SessionID != "sess-caller" {
		t.Errorf("SessionID = %q, want sess-caller (flag is on)", spy.lastInput.SessionID)
	}
}

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
