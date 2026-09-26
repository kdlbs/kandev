package dashboard_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
)

// seedTwoStepWorkflowTask creates a two-step workflow (stepAID at
// position 0, non-terminal; stepBID at position 1, named "Done" so
// IsTaskWorkflowStepTerminal reports it terminal) and a task row that
// carries an explicit tasks.workflow_id, pointing at stepAID. Unlike
// insertTestTask/insertTestTaskAtNonTerminalStep, this sets
// tasks.workflow_id so the workflow-scoped participant read (the fix
// under test) has a workflow to join across steps with — the existing
// dashboard helpers all leave it at the schema default ”, which only
// exercises the any-step fallback, not the JOIN-scoped branch.
func seedTwoStepWorkflowTask(
	t *testing.T, db sqlxExecutor, id, wsID, title, state, workflowID, stepAID, stepBID string,
) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO workflows (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, 'Test Workflow', datetime('now'), datetime('now'))
	`, workflowID, wsID); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES (?, ?, 'Work', 0, datetime('now'), datetime('now'))
	`, stepAID, workflowID); err != nil {
		t.Fatalf("seed step A: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES (?, ?, 'Done', 1, datetime('now'), datetime('now'))
	`, stepBID, workflowID); err != nil {
		t.Fatalf("seed step B: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO tasks (
			id, workspace_id, title, state, priority, identifier,
			workflow_id, workflow_step_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'medium', ?, ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, title, state, id, workflowID, stepAID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

// moveTaskToWorkflowStep updates a task's current step in place,
// mirroring a workflow step-move without re-registering participants.
func moveTaskToWorkflowStep(t *testing.T, db sqlxExecutor, taskID, stepID string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, stepID, taskID); err != nil {
		t.Fatalf("move task to step: %v", err)
	}
}

// seedRunnerSeat inserts a 'runner' participant row for taskID at
// stepID, the shape RunnerProjection's per-task fallback tier resolves
// as the assignee regardless of which step the task currently stands
// at (see internal/office/repository/sqlite/base.go).
func seedRunnerSeat(t *testing.T, db sqlxExecutor, stepID, taskID, agentID string) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'runner', ?, 0, 0)
	`, "p-runner-"+taskID, stepID, taskID, agentID); err != nil {
		t.Fatalf("insert runner participant: %v", err)
	}
}

// TestApproveTask_NotForbiddenAfterStepMove covers AC-OFFICE-SEAT-READ-SCOPE-001.1:
// an approver seated while the task stood at a non-terminal step must still
// be recognized as a participant (resolveDeciderRole via ListAllTaskParticipants)
// once the task has moved to a later step of the same workflow.
func TestApproveTask_NotForbiddenAfterStepMove(t *testing.T) {
	deps := newTestDeps(t)
	seedTwoStepWorkflowTask(t, deps.db, "mv1", "ws-g", "MV", "in_review",
		"wf-mv1", "step-mv1-a", "step-mv1-b")
	mustAddParticipant(t, deps, "mv1", "agent-A", models.ParticipantRoleApprover)

	moveTaskToWorkflowStep(t, deps.db, "mv1", "step-mv1-b")

	d, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-A", "mv1", "lgtm")
	if err != nil {
		t.Fatalf("ApproveTask after step move: %v", err)
	}
	if d == nil || d.Decision != models.DecisionApproved {
		t.Fatalf("decision = %+v", d)
	}
}

// TestUpdateTaskStatus_GatedWhenApproverSeatedBeforeStepMove covers
// AC-OFFICE-SEAT-READ-SCOPE-001.1's approval-gate consequence: an approver
// seated at a non-terminal step, still undecided, must still block a
// "done" transition after the task has moved to the workflow's terminal
// step. Before the fix, ListTaskParticipants(role=approver) only looked
// at the task's *current* step, so pendingApprovers saw an empty seat
// list and applyApprovalGate treated that as "nothing to wait for" —
// silently completing the task despite the undecided approver.
func TestUpdateTaskStatus_GatedWhenApproverSeatedBeforeStepMove(t *testing.T) {
	deps := newTestDeps(t)
	seedTwoStepWorkflowTask(t, deps.db, "mv2", "ws-g", "MV2", "in_progress",
		"wf-mv2", "step-mv2-a", "step-mv2-b")
	mustAddParticipant(t, deps, "mv2", "agent-A", models.ParticipantRoleApprover)

	moveTaskToWorkflowStep(t, deps.db, "mv2", "step-mv2-b")

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "mv2",
		NewStatus: "done",
	})
	var pending *dashboard.ApprovalsPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("err = %v, want *ApprovalsPendingError (approval gate bypassed)", err)
	}
	if pending.Reason != dashboard.ApprovalGateReasonApprovals {
		t.Errorf("Reason = %q, want %q", pending.Reason, dashboard.ApprovalGateReasonApprovals)
	}
	if len(pending.Pending) != 1 || pending.Pending[0] != "agent-A" {
		t.Fatalf("pending = %v, want [agent-A]", pending.Pending)
	}
	state := readTaskState(t, deps, "mv2")
	if state != "REVIEW" {
		t.Errorf("state = %q, want REVIEW (gate must redirect, not silently complete)", state)
	}
}

// TestApproveTask_QueuesReadyToCloseOnFinalApproval_AfterStepMove pins the
// dedup half of the fix: an approver seated before a step move and one
// seated at the post-move step must both be counted, and ready_to_close
// must wait for both rather than firing (or double-firing) on the first.
func TestApproveTask_QueuesReadyToCloseOnFinalApproval_AfterStepMove(t *testing.T) {
	deps := newTestDeps(t)
	seedTwoStepWorkflowTask(t, deps.db, "mv3", "ws-g", "MV3", "in_review",
		"wf-mv3", "step-mv3-a", "step-mv3-b")
	seedRunnerSeat(t, deps.db, "step-mv3-a", "mv3", "asg-mv3")
	mustAddParticipant(t, deps, "mv3", "agent-A", models.ParticipantRoleApprover)

	moveTaskToWorkflowStep(t, deps.db, "mv3", "step-mv3-b")
	mustAddParticipant(t, deps, "mv3", "agent-B", models.ParticipantRoleApprover)

	q := &stubApprovalQueuer{}
	deps.svc.SetApprovalReactivityQueuer(q)

	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-A", "mv3", ""); err != nil {
		t.Fatalf("first approve (pre-move seat): %v", err)
	}
	if len(q.runs) != 0 {
		t.Fatalf("expected no run yet with agent-B still pending, got %v", q.runs)
	}
	if _, err := deps.svc.ApproveTask(context.Background(),
		models.DeciderTypeAgent, "agent-B", "mv3", ""); err != nil {
		t.Fatalf("second approve (post-move seat): %v", err)
	}
	if len(q.runs) != 1 {
		t.Fatalf("runs = %d, want 1: %#v", len(q.runs), q.runs)
	}
	if q.runs[0].Reason != "task_ready_to_close" {
		t.Fatalf("reason = %s", q.runs[0].Reason)
	}
}

// TestInbox_TaskReviewRequest_SurvivesStepMove is review round 1's
// test-rigor finding R1-2: the card explicitly required "the inbox item
// survives" a step move, but before this test only the repository-level
// slate tests covered that transitively. buildReviewRequestItem's own call
// chain (ListAllTaskParticipants -> viewerRoles -> viewerNeedsDecision) was
// never exercised end to end through the dashboard service. Seat a reviewer
// at step A, move to step B, and assert the agent's review-request inbox
// item is still returned.
func TestInbox_TaskReviewRequest_SurvivesStepMove(t *testing.T) {
	deps := newTestDeps(t)
	seedTwoStepWorkflowTask(t, deps.db, "ib-mv", "ws-i", "IB", "in_review",
		"wf-ib-mv", "step-ib-mv-a", "step-ib-mv-b")
	mustAddParticipant(t, deps, "ib-mv", "agent-A", models.ParticipantRoleReviewer)

	moveTaskToWorkflowStep(t, deps.db, "ib-mv", "step-ib-mv-b")

	items, err := deps.svc.GetAgentInboxItems(context.Background(), "ws-i", "agent-A")
	if err != nil {
		t.Fatalf("GetAgentInboxItems: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Type == "task_review_request" && it.EntityID == "ib-mv" {
			found = true
		}
	}
	if !found {
		t.Fatalf("review request item for ib-mv missing after step move: %#v", items)
	}
}
