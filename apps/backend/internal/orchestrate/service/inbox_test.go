package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestGetInboxItems_IncludesApprovals(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	// Create a pending approval.
	a := &models.Approval{
		WorkspaceID:                "ws-1",
		Type:                       "hire_agent",
		RequestedByAgentInstanceID: "agent-1",
		Payload:                    `{"name":"qa-bot"}`,
	}
	if err := svc.CreateApprovalWithActivity(ctx, a); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	items, err := svc.GetInboxItems(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetInboxItems: %v", err)
	}

	found := false
	for _, item := range items {
		if item.Type == "approval" && item.EntityID == a.ID {
			found = true
			if item.Status != "pending" {
				t.Errorf("status = %q, want pending", item.Status)
			}
		}
	}
	if !found {
		t.Error("expected approval in inbox items")
	}
}

func TestGetInboxItems_IncludesBudgetAlerts(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Log a budget alert activity entry.
	svc.LogActivity(ctx, "ws-1", "system", "budget_checker",
		"budget.alert", "agent", "agent-1", `{"spend":850,"limit":1000}`)

	items, err := svc.GetInboxItems(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetInboxItems: %v", err)
	}

	found := false
	for _, item := range items {
		if item.Type == "budget_alert" {
			found = true
		}
	}
	if !found {
		t.Error("expected budget_alert in inbox items")
	}
}

func TestGetInboxItems_IncludesAgentErrors(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Log an agent error activity entry.
	svc.LogActivity(ctx, "ws-1", "system", "lifecycle",
		"agent.error", "agent", "agent-1", `{"error":"session failed"}`)

	items, err := svc.GetInboxItems(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetInboxItems: %v", err)
	}

	found := false
	for _, item := range items {
		if item.Type == "agent_error" {
			found = true
		}
	}
	if !found {
		t.Error("expected agent_error in inbox items")
	}
}

func TestGetInboxCount(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	// Create a pending approval.
	a := &models.Approval{
		WorkspaceID:                "ws-1",
		Type:                       "hire_agent",
		RequestedByAgentInstanceID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, a); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	// Log a budget alert.
	svc.LogActivity(ctx, "ws-1", "system", "budget_checker",
		"budget.alert", "agent", "agent-1", `{}`)

	count, err := svc.GetInboxCount(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetInboxCount: %v", err)
	}
	// 1 pending approval + 1 budget alert = at least 2.
	if count < 2 {
		t.Errorf("count = %d, want >= 2", count)
	}
}

func TestGetInboxItems_Empty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	items, err := svc.GetInboxItems(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetInboxItems: %v", err)
	}
	if items == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(items) != 0 {
		t.Errorf("count = %d, want 0", len(items))
	}
}
