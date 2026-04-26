package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestCheckBudget_UnderThreshold(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	// Create budget policy: $10 limit (1000 cents), 80% alert threshold.
	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-1",
		LimitCents:        1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Record small cost event (well under threshold).
	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		100_000, 0, 0, // 100k * 300 / 1M = 30 cents
	)

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].AlertFired {
		t.Error("alert should not fire under threshold")
	}
	if results[0].LimitExceed {
		t.Error("limit should not be exceeded")
	}
	if results[0].AgentPaused {
		t.Error("agent should not be paused")
	}
}

func TestCheckBudget_OverThreshold_Alert(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-1",
		LimitCents:        1000,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "notify_only",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Record cost that exceeds 80% but not 100%: 850 cents.
	// 850 cents = 2_833_333 input tokens at 300/M
	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		2_833_333, 0, 0,
	)

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].AlertFired {
		t.Error("alert should fire at 85% of limit")
	}
	if results[0].LimitExceed {
		t.Error("limit should not be exceeded")
	}
}

func TestCheckBudget_OverLimit_PauseAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-1")

	policy := &models.BudgetPolicy{
		WorkspaceID:       "ws-1",
		ScopeType:         "agent",
		ScopeID:           "agent-1",
		LimitCents:        500,
		Period:            "monthly",
		AlertThresholdPct: 80,
		ActionOnExceed:    "pause_agent",
	}
	if err := svc.CreateBudgetPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	// Record cost exceeding the limit: 600 cents.
	// 600 = 2_000_000 input tokens * 300 / 1M
	_, _ = svc.RecordCostEvent(ctx,
		"sess-1", "task-1", "agent-1", "proj-1",
		"claude-sonnet-4", "anthropic",
		2_000_000, 0, 0,
	)

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].LimitExceed {
		t.Error("limit should be exceeded")
	}
	if !results[0].AgentPaused {
		t.Error("agent should be paused")
	}

	// Verify the agent's status changed.
	agent, err := svc.GetAgentInstance(ctx, "agent-1")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != "paused" {
		t.Errorf("status = %q, want paused", agent.Status)
	}
	if agent.PauseReason != "budget_exceeded" {
		t.Errorf("pause_reason = %q, want budget_exceeded", agent.PauseReason)
	}
}

func TestCheckBudget_NoPolicies(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	results, err := svc.CheckBudget(ctx, "ws-1", "agent-1", "proj-1")
	if err != nil {
		t.Fatalf("CheckBudget: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %d", len(results))
	}
}
