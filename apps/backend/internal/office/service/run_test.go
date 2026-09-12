package service_test

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// windowedDedupCounterValue reads the current value of one
// office_run_dedup_total{reason,kind=windowed,queue=runs} label, or 0 if the
// label has never been reported. Callers snapshot before/after and assert
// the exact delta.
func windowedDedupCounterValue(t *testing.T, reason string) int64 {
	t.Helper()
	v := expvar.Get("office_run_dedup_total")
	if v == nil {
		t.Fatalf("expvar map office_run_dedup_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("office_run_dedup_total is not a *expvar.Map")
	}
	iv := m.Get("reason=" + reason + ";kind=windowed;queue=runs")
	if iv == nil {
		return 0
	}
	i, ok := iv.(*expvar.Int)
	if !ok {
		t.Fatalf("counter value for reason=%s;kind=windowed;queue=runs is not *expvar.Int", reason)
	}
	return i.Value()
}

func TestQueueRun_Basic(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	_, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, "key-1")
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
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", key); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	// Second enqueue with same key should be silently dropped.
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", key); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	reqs, _ := svc.ListRuns(ctx, "ws-1")
	if len(reqs) != 1 {
		t.Errorf("want 1 run (idempotent), got %d", len(reqs))
	}
}

// TestQueueRun_Idempotency_ReportsWindowedDedupOutcome closes a phantom-green
// gap: TestQueueRun_Idempotency above only asserted the resulting row count,
// so deleting queueRunInline's ReportWindowedDedup call would leave it green
// while AC-OFFICE-RUN-DEDUP-004.1's observability contract silently went
// unmet on this queue implementation. Asserts the outcome value and the
// office_run_dedup_total counter directly.
func TestQueueRun_Idempotency_ReportsWindowedDedupOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	key := "idem-key-outcome"
	first, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", key)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %v, want QueueOutcomeQueued", first)
	}

	before := windowedDedupCounterValue(t, service.RunReasonTaskAssigned)

	second, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", key)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if second != runsservice.QueueOutcomeDeduped {
		t.Fatalf("second outcome = %v, want QueueOutcomeDeduped", second)
	}

	after := windowedDedupCounterValue(t, service.RunReasonTaskAssigned)
	if after != before+1 {
		t.Fatalf("office_run_dedup_total{%s,windowed,runs} = %d, want %d", service.RunReasonTaskAssigned, after, before+1)
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

	_, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "")
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

	_, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "")
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
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskComment, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskComment, `{"task_id":"t1"}`, ""); err != nil {
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
