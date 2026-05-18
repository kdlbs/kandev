package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func TestQueueRun_Basic(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, "key-1")
	if err != nil {
		t.Fatalf("queue run: %v", err)
	}

	reqs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 run, got %d", len(reqs))
	}
	if reqs[0].Reason != service.RunReasonTaskAssigned {
		t.Errorf("reason = %q, want %q", reqs[0].Reason, service.RunReasonTaskAssigned)
	}
	if reqs[0].Status != service.RunStatusQueued {
		t.Errorf("status = %q, want %q", reqs[0].Status, service.RunStatusQueued)
	}
}

func TestQueueRun_Idempotency(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	key := "idem-key-1"
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", key); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	// Second enqueue with same key should be silently dropped.
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", key); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	reqs, _ := svc.ListRuns(ctx, "ws-1")
	if len(reqs) != 1 {
		t.Errorf("want 1 run (idempotent), got %d", len(reqs))
	}
}

func TestQueueRun_SkipsPausedAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Pause the agent.
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause agent: %v", err)
	}

	err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "")
	if err == nil {
		t.Fatal("expected error for paused agent")
	}

	reqs, _ := svc.ListRuns(ctx, "ws-1")
	if len(reqs) != 0 {
		t.Errorf("want 0 runs for paused agent, got %d", len(reqs))
	}
}

func TestQueueRun_SkipsStoppedAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusStopped, ""); err != nil {
		t.Fatalf("stop agent: %v", err)
	}

	err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "")
	if err == nil {
		t.Fatal("expected error for stopped agent")
	}
}

func TestQueueRun_Coalesce(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Two runs with the same agent + reason within coalesce window should merge.
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskComment, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskComment, `{"task_id":"t2"}`, ""); err != nil {
		t.Fatalf("second: %v", err)
	}

	reqs, _ := svc.ListRuns(ctx, "ws-1")
	if len(reqs) != 1 {
		t.Fatalf("want 1 coalesced run, got %d", len(reqs))
	}
	if reqs[0].CoalescedCount != 2 {
		t.Errorf("coalesced_count = %d, want 2", reqs[0].CoalescedCount)
	}
}
