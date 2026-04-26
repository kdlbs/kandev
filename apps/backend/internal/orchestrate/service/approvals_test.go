package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestCreateApprovalWithActivity(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	approval := &models.Approval{
		WorkspaceID:                "ws-1",
		Type:                       "hire_agent",
		RequestedByAgentInstanceID: "agent-1",
		Payload:                    `{"name":"qa-bot"}`,
	}

	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	if approval.ID == "" {
		t.Error("approval ID should be set")
	}
	if approval.Status != "pending" {
		t.Errorf("status = %q, want pending", approval.Status)
	}

	// Verify activity was logged.
	entries, err := svc.ListActivity(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected activity entry for approval creation")
	}
}

func TestDecideApproval_Approve(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	approval := &models.Approval{
		WorkspaceID:                "ws-1",
		Type:                       "hire_agent",
		RequestedByAgentInstanceID: "agent-1",
		Payload:                    `{"name":"qa-bot"}`,
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	decided, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", "Looks good")
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if decided.Status != "approved" {
		t.Errorf("status = %q, want approved", decided.Status)
	}
	if decided.DecidedBy != "user-1" {
		t.Errorf("decided_by = %q, want user-1", decided.DecidedBy)
	}
	if decided.DecisionNote != "Looks good" {
		t.Errorf("note = %q, want 'Looks good'", decided.DecisionNote)
	}
	if decided.DecidedAt == nil {
		t.Error("decided_at should be set")
	}
}

func TestDecideApproval_Reject(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	approval := &models.Approval{
		WorkspaceID:                "ws-1",
		Type:                       "task_review",
		RequestedByAgentInstanceID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	decided, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-1", "Needs work")
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if decided.Status != "rejected" {
		t.Errorf("status = %q, want rejected", decided.Status)
	}
}

func TestDecideApproval_AlreadyDecided(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	approval := &models.Approval{
		WorkspaceID:                "ws-1",
		Type:                       "hire_agent",
		RequestedByAgentInstanceID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	// First decision succeeds.
	if _, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", ""); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	// Second decision fails.
	if _, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-2", ""); err == nil {
		t.Error("expected error when deciding already-decided approval")
	}
}

func TestGetPendingApprovals(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	// Create 3 approvals, decide 1.
	for i := 0; i < 3; i++ {
		a := &models.Approval{
			WorkspaceID:                "ws-1",
			Type:                       "hire_agent",
			RequestedByAgentInstanceID: "agent-1",
		}
		if err := svc.CreateApprovalWithActivity(ctx, a); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if i == 0 {
			if _, err := svc.DecideApproval(ctx, a.ID, "approved", "user-1", ""); err != nil {
				t.Fatalf("decide: %v", err)
			}
		}
	}

	pending, err := svc.GetPendingApprovals(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetPendingApprovals: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("count = %d, want 2", len(pending))
	}
}
