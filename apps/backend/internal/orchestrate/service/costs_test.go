package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func createTestAgent(t *testing.T, svc *service.Service, wsID, agentID string) {
	t.Helper()
	agent := &models.AgentInstance{
		ID:          agentID,
		WorkspaceID: wsID,
		Name:        "test-" + agentID,
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create test agent: %v", err)
	}
}

func TestRecordCostEvent_KnownModel(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create an agent instance so cost aggregation JOIN works.
	createTestAgent(t, svc, "ws-1", "agent-1")

	event, err := svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		1_000_000, 500_000, 100_000,
	)
	if err != nil {
		t.Fatalf("RecordCostEvent: %v", err)
	}
	if event.CostCents == 0 {
		t.Error("expected non-zero cost for known model")
	}
	// 1M * 300 / 1M = 300
	// 500k * 30 / 1M = 15
	// 100k * 1500 / 1M = 150
	// total = 465
	if event.CostCents != 465 {
		t.Errorf("cost_cents = %d, want 465", event.CostCents)
	}
}

func TestRecordCostEvent_UnknownModel(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	event, err := svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"unknown-model-xyz", "unknown",
		500_000, 0, 100_000,
	)
	if err != nil {
		t.Fatalf("RecordCostEvent: %v", err)
	}
	if event.CostCents != 0 {
		t.Errorf("cost_cents = %d, want 0 for unknown model", event.CostCents)
	}
}

func TestGetCostSummary(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		1_000_000, 0, 0,
	)
	_, _ = svc.RecordCostEvent(ctx,
		"sess-2", "task-2", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		2_000_000, 0, 0,
	)

	total, err := svc.GetCostSummary(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetCostSummary: %v", err)
	}
	// 1M*300/1M=300, 2M*300/1M=600 => 900
	if total != 900 {
		t.Errorf("total = %d, want 900", total)
	}
}
