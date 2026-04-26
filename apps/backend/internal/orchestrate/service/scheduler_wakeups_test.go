package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestClaimNextWakeup_ReturnsNilWhenEmpty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	req, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if req != nil {
		t.Errorf("expected nil, got %+v", req)
	}
}

func TestClaimNextWakeup_ClaimsQueued(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	req, err := svc.ClaimNextWakeup(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if req == nil {
		t.Fatal("expected a wakeup, got nil")
	}
	if req.AgentInstanceID != agent.ID {
		t.Errorf("agent = %q, want %q", req.AgentInstanceID, agent.ID)
	}
}

func TestProcessWakeupGuard_AllowsActiveAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	wakeup := &models.WakeupRequest{AgentInstanceID: agent.ID}
	ok, err := svc.ProcessWakeupGuard(ctx, wakeup)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !ok {
		t.Error("expected guard to allow idle agent")
	}
}

func TestProcessWakeupGuard_BlocksPausedAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	wakeup := &models.WakeupRequest{AgentInstanceID: agent.ID}
	ok, err := svc.ProcessWakeupGuard(ctx, wakeup)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if ok {
		t.Error("expected guard to block paused agent")
	}
}

func TestFinishAndFailWakeup(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, "{}", "k1"); err != nil {
		t.Fatalf("queue: %v", err)
	}

	req, _ := svc.ClaimNextWakeup(ctx)
	if req == nil {
		t.Fatal("expected claimed wakeup")
	}

	if err := svc.FinishWakeup(ctx, req.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Should be empty now.
	next, _ := svc.ClaimNextWakeup(ctx)
	if next != nil {
		t.Error("expected no more wakeups")
	}
}
