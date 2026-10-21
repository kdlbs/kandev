package dashboard_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// @covers AC-OFFICE-HUMAN-DONE-002.1
func TestPendingApproversForCompletion_ListsUnapprovedApprovers(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "pa1", "ws-g", "PA", "in_progress", 2)
	mustAddParticipant(t, deps, "pa1", "agent-A", models.ParticipantRoleApprover)

	pending, err := deps.svc.PendingApproversForCompletion(context.Background(), "pa1", "")
	if err != nil {
		t.Fatalf("PendingApproversForCompletion: %v", err)
	}
	if len(pending) != 1 || pending[0].AgentProfileID != "agent-A" {
		t.Fatalf("pending = %+v, want agent-A", pending)
	}
}

func TestPendingApproversForCompletion_NoApprovers_Empty(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "pa2", "ws-g", "PA", "in_progress", 2)

	pending, err := deps.svc.PendingApproversForCompletion(context.Background(), "pa2", "")
	if err != nil {
		t.Fatalf("PendingApproversForCompletion: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want none", pending)
	}
}

// @covers AC-OFFICE-HUMAN-DONE-002.1
func TestPendingApproversForCompletion_CountsTemplateApproverAtDestinationStep(t *testing.T) {
	deps := newTestDeps(t)
	seedTwoStepWorkflowTask(t, deps.db, "pa3", "ws-g", "PA", "in_progress",
		"wf-pa3", "step-pa3-work", "step-pa3-done")
	if _, err := deps.db.Exec(`
		INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('p-tpl-pa3', 'step-pa3-done', '', 'approver', 'agent-final', 1, 0)
	`); err != nil {
		t.Fatalf("insert template approver: %v", err)
	}

	atCurrent, err := deps.svc.PendingApproversForCompletion(context.Background(), "pa3", "")
	if err != nil {
		t.Fatalf("PendingApproversForCompletion(current): %v", err)
	}
	if len(atCurrent) != 0 {
		t.Fatalf("pending at current step = %+v, want none", atCurrent)
	}
	atDestination, err := deps.svc.PendingApproversForCompletion(context.Background(), "pa3", "step-pa3-done")
	if err != nil {
		t.Fatalf("PendingApproversForCompletion(destination): %v", err)
	}
	if len(atDestination) != 1 || atDestination[0].AgentProfileID != "agent-final" {
		t.Fatalf("pending at destination = %+v, want agent-final", atDestination)
	}
}
