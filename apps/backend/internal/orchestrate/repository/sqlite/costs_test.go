package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestCostEvent_CreateAndList(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Need an agent instance to join against
	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "cost-agent",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		Permissions:        "{}",
		DesiredSkills:      "[]",
		ExecutorPreference: "{}",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	event := &models.CostEvent{
		SessionID:       "session-1",
		TaskID:          "task-1",
		AgentInstanceID: agent.ID,
		Model:           "claude-4-sonnet",
		Provider:        "anthropic",
		TokensIn:        1000,
		TokensOut:       500,
		CostCents:       10,
		OccurredAt:      time.Now().UTC(),
	}
	if err := repo.CreateCostEvent(ctx, event); err != nil {
		t.Fatalf("create cost: %v", err)
	}

	costs, err := repo.ListCostEvents(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list costs: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("cost count = %d, want 1", len(costs))
	}
	if costs[0].CostCents != 10 {
		t.Errorf("cost_cents = %d, want 10", costs[0].CostCents)
	}
}

func TestCostBreakdowns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "breakdown-agent",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		Permissions:        "{}",
		DesiredSkills:      "[]",
		ExecutorPreference: "{}",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	for i := 0; i < 3; i++ {
		event := &models.CostEvent{
			AgentInstanceID: agent.ID,
			Model:           "claude-4-sonnet",
			ProjectID:       "proj-1",
			CostCents:       5,
			OccurredAt:      time.Now().UTC(),
		}
		if err := repo.CreateCostEvent(ctx, event); err != nil {
			t.Fatalf("create cost %d: %v", i, err)
		}
	}

	byAgent, err := repo.GetCostsByAgent(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by agent: %v", err)
	}
	if len(byAgent) != 1 || byAgent[0].TotalCents != 15 {
		t.Errorf("by agent: got %+v", byAgent)
	}

	byProject, err := repo.GetCostsByProject(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by project: %v", err)
	}
	if len(byProject) != 1 || byProject[0].TotalCents != 15 {
		t.Errorf("by project: got %+v", byProject)
	}

	byModel, err := repo.GetCostsByModel(ctx, "ws-1")
	if err != nil {
		t.Fatalf("by model: %v", err)
	}
	if len(byModel) != 1 || byModel[0].GroupKey != "claude-4-sonnet" {
		t.Errorf("by model: got %+v", byModel)
	}
}
