package dashboard_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// TestRecordAgentDecision_TwoRejectionRoundsQueueDistinctRuns is the
// agent-path counterpart to TestRequestTaskChanges_TwoRoundsQueueDistinctRuns:
// two rejected verdicts against the same task within the idempotency window
// each queue their own assignee run, keyed off the decision that produced
// them rather than the static (reason, task, agent) tuple.
func TestRecordAgentDecision_TwoRejectionRoundsQueueDistinctRuns(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskWithAssignee(t, deps.db, "rej2", "ws-d", "REJ2", "in_review", 2, "asg-rej2")
	mustAddParticipant(t, deps, "rej2", "agent-rev", models.ParticipantRoleReviewer)

	q := &stubApprovalQueuer{}
	deps.svc.SetApprovalReactivityQueuer(q)

	first, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "rej2",
		AgentProfileID: "agent-rev",
		Decision:       engine.DecisionRejected,
		Reason:         "round one",
	})
	if err != nil {
		t.Fatalf("first RecordAgentDecision: %v", err)
	}
	second, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID:         "rej2",
		AgentProfileID: "agent-rev",
		Decision:       engine.DecisionRejected,
		Reason:         "round two",
	})
	if err != nil {
		t.Fatalf("second RecordAgentDecision: %v", err)
	}
	if len(q.runs) != 2 {
		t.Fatalf("runs = %d, want 2: %#v", len(q.runs), q.runs)
	}
	if first.DecisionID == second.DecisionID {
		t.Fatalf("engine returned identical DecisionIDs for two separate decisions: %q", first.DecisionID)
	}
	if want := "decision:" + first.DecisionID; q.runs[0].IdempotencyKey != want {
		t.Errorf("runs[0].IdempotencyKey = %q, want %q", q.runs[0].IdempotencyKey, want)
	}
	if want := "decision:" + second.DecisionID; q.runs[1].IdempotencyKey != want {
		t.Errorf("runs[1].IdempotencyKey = %q, want %q", q.runs[1].IdempotencyKey, want)
	}
}

// TestRequestTaskChanges_TwoRoundsQueueDistinctRuns verifies that two
// changes-requested decisions against the same task within the
// idempotency window each queue their own assignee run: the
// idempotency key is derived from the decision, not from the static
// (reason, task, agent) tuple, so a second round is never deduped away.
func TestRequestTaskChanges_TwoRoundsQueueDistinctRuns(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskWithAssignee(t, deps.db, "ch2", "ws-d", "C2", "in_review", 2, "asg-1")
	mustAddParticipant(t, deps, "ch2", "agent-rev", models.ParticipantRoleReviewer)

	q := &stubApprovalQueuer{}
	deps.svc.SetApprovalReactivityQueuer(q)

	first, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeAgent, "agent-rev", "ch2", "round one")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	second, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeAgent, "agent-rev", "ch2", "round two")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if len(q.runs) != 2 {
		t.Fatalf("runs = %d, want 2: %#v", len(q.runs), q.runs)
	}
	if first.ID == second.ID {
		t.Fatalf("engine returned identical decision IDs for two separate decisions: %q", first.ID)
	}
	if want := "decision:" + first.ID; q.runs[0].IdempotencyKey != want {
		t.Errorf("runs[0].IdempotencyKey = %q, want %q", q.runs[0].IdempotencyKey, want)
	}
	if want := "decision:" + second.ID; q.runs[1].IdempotencyKey != want {
		t.Errorf("runs[1].IdempotencyKey = %q, want %q", q.runs[1].IdempotencyKey, want)
	}
}

// TestRequestTaskChanges_HumanCallerTwoRoundsQueueDistinctRuns is the
// DeciderTypeUser counterpart to TestRequestTaskChanges_TwoRoundsQueueDistinctRuns:
// resolveDeciderRole and resolveParticipantID take a different branch for a
// human caller (implicit approver, sentinel participant ID, no participant
// row), so the agent-decider coverage above does not exercise it.
func TestRequestTaskChanges_HumanCallerTwoRoundsQueueDistinctRuns(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskWithAssignee(t, deps.db, "chu2", "ws-d", "CHU2", "in_review", 2, "asg-1")

	q := &stubApprovalQueuer{}
	deps.svc.SetApprovalReactivityQueuer(q)

	first, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeUser, "user", "chu2", "round one")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	second, err := deps.svc.RequestTaskChanges(context.Background(),
		models.DeciderTypeUser, "user", "chu2", "round two")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if len(q.runs) != 2 {
		t.Fatalf("runs = %d, want 2: %#v", len(q.runs), q.runs)
	}
	if first.ID == second.ID {
		t.Fatalf("engine returned identical decision IDs for two separate decisions: %q", first.ID)
	}
	if want := "decision:" + first.ID; q.runs[0].IdempotencyKey != want {
		t.Errorf("runs[0].IdempotencyKey = %q, want %q", q.runs[0].IdempotencyKey, want)
	}
	if want := "decision:" + second.ID; q.runs[1].IdempotencyKey != want {
		t.Errorf("runs[1].IdempotencyKey = %q, want %q", q.runs[1].IdempotencyKey, want)
	}
}
