package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func TestClaimNextRun_ReturnsNilWhenEmpty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	req, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if req != nil {
		t.Errorf("expected nil, got %+v", req)
	}
}

func TestClaimNextRun_ClaimsQueued(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	req, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if req == nil {
		t.Fatal("expected a run, got nil")
	}
	if req.AgentProfileID != agent.ID {
		t.Errorf("agent = %q, want %q", req.AgentProfileID, agent.ID)
	}
}

func TestProcessRunGuard_AllowsActiveAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	run := &models.Run{AgentProfileID: agent.ID}
	ok, err := svc.ProcessRunGuard(ctx, run)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !ok {
		t.Error("expected guard to allow idle agent")
	}
}

func TestProcessRunGuard_BlocksPausedAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	run := &models.Run{AgentProfileID: agent.ID}
	ok, err := svc.ProcessRunGuard(ctx, run)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if ok {
		t.Error("expected guard to block paused agent")
	}
}

func TestFinishAndFailRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k1"); err != nil {
		t.Fatalf("queue: %v", err)
	}

	req, _ := svc.ClaimNextRun(ctx)
	if req == nil {
		t.Fatal("expected claimed run")
	}

	if err := svc.FinishRun(ctx, req.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Should be empty now.
	next, _ := svc.ClaimNextRun(ctx)
	if next != nil {
		t.Error("expected no more runs")
	}
}
