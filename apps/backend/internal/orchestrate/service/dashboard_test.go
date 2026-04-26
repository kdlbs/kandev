package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestGetDashboardData_Empty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	data, err := svc.GetDashboardData(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if data.AgentCount != 0 {
		t.Errorf("agent_count = %d, want 0", data.AgentCount)
	}
	if data.PendingApprovals != 0 {
		t.Errorf("pending_approvals = %d, want 0", data.PendingApprovals)
	}
	if data.RecentActivity == nil {
		t.Error("recent_activity should be non-nil")
	}
}

func TestGetDashboardData_WithAgentsAndApprovals(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create agents with different statuses.
	createTestAgent(t, svc, "ws-1", "agent-1")
	createTestAgent(t, svc, "ws-1", "agent-2")
	createTestAgent(t, svc, "ws-1", "agent-3")

	// Set agent-2 to working.
	agent2, _ := svc.GetAgentInstance(ctx, "agent-2")
	agent2.Status = models.AgentStatusWorking
	if err := svc.UpdateAgentInstance(ctx, agent2); err != nil {
		t.Fatalf("update agent: %v", err)
	}

	// Set agent-3 to paused.
	agent3, _ := svc.GetAgentInstance(ctx, "agent-3")
	agent3.Status = models.AgentStatusPaused
	agent3.PauseReason = "budget_exceeded"
	if err := svc.UpdateAgentInstance(ctx, agent3); err != nil {
		t.Fatalf("update agent: %v", err)
	}

	// Create a pending approval.
	approval := &models.Approval{
		WorkspaceID:                "ws-1",
		Type:                       "hire_agent",
		RequestedByAgentInstanceID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	// Log some activity.
	svc.LogActivity(ctx, "ws-1", "user", "user-1", "task.created", "task", "t-1", `{}`)

	data, err := svc.GetDashboardData(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}

	if data.AgentCount != 3 {
		t.Errorf("agent_count = %d, want 3", data.AgentCount)
	}
	if data.RunningCount != 1 {
		t.Errorf("running_count = %d, want 1", data.RunningCount)
	}
	if data.PausedCount != 1 {
		t.Errorf("paused_count = %d, want 1", data.PausedCount)
	}
	if data.PendingApprovals != 1 {
		t.Errorf("pending_approvals = %d, want 1", data.PendingApprovals)
	}
	if len(data.RecentActivity) == 0 {
		t.Error("expected recent activity entries")
	}
}
