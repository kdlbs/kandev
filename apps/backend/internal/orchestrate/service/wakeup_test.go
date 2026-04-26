package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestQueueWakeup_Basic(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, `{"task_id":"t1"}`, "key-1")
	if err != nil {
		t.Fatalf("queue wakeup: %v", err)
	}

	reqs, err := svc.ListWakeupRequests(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 wakeup, got %d", len(reqs))
	}
	if reqs[0].Reason != service.WakeupReasonTaskAssigned {
		t.Errorf("reason = %q, want %q", reqs[0].Reason, service.WakeupReasonTaskAssigned)
	}
	if reqs[0].Status != service.WakeupStatusQueued {
		t.Errorf("status = %q, want %q", reqs[0].Status, service.WakeupStatusQueued)
	}
}

func TestQueueWakeup_Idempotency(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	key := "idem-key-1"
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, "{}", key); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	// Second enqueue with same key should be silently dropped.
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, "{}", key); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	reqs, _ := svc.ListWakeupRequests(ctx, "ws-1")
	if len(reqs) != 1 {
		t.Errorf("want 1 wakeup (idempotent), got %d", len(reqs))
	}
}

func TestQueueWakeup_SkipsPausedAgent(t *testing.T) {
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

	err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, "{}", "")
	if err == nil {
		t.Fatal("expected error for paused agent")
	}

	reqs, _ := svc.ListWakeupRequests(ctx, "ws-1")
	if len(reqs) != 0 {
		t.Errorf("want 0 wakeups for paused agent, got %d", len(reqs))
	}
}

func TestQueueWakeup_SkipsStoppedAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusStopped, ""); err != nil {
		t.Fatalf("stop agent: %v", err)
	}

	err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskAssigned, "{}", "")
	if err == nil {
		t.Fatal("expected error for stopped agent")
	}
}

func TestQueueWakeup_Coalesce(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Two wakeups with the same agent + reason within coalesce window should merge.
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskComment, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := svc.QueueWakeup(ctx, agent.ID, service.WakeupReasonTaskComment, `{"task_id":"t2"}`, ""); err != nil {
		t.Fatalf("second: %v", err)
	}

	reqs, _ := svc.ListWakeupRequests(ctx, "ws-1")
	if len(reqs) != 1 {
		t.Fatalf("want 1 coalesced wakeup, got %d", len(reqs))
	}
	if reqs[0].CoalescedCount != 2 {
		t.Errorf("coalesced_count = %d, want 2", reqs[0].CoalescedCount)
	}
}
