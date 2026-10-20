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

	pending, err := deps.svc.PendingApproversForCompletion(context.Background(), "pa1")
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

	pending, err := deps.svc.PendingApproversForCompletion(context.Background(), "pa2")
	if err != nil {
		t.Fatalf("PendingApproversForCompletion: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want none", pending)
	}
}
