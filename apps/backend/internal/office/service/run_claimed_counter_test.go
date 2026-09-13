package service_test

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func runClaimedExpvarInt(t *testing.T, key string) int64 {
	t.Helper()
	v := expvar.Get("office_loop_run_claimed_total")
	if v == nil {
		t.Fatal("expvar map office_loop_run_claimed_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatal("office_loop_run_claimed_total is not a Map")
	}
	sub := m.Get(key)
	if sub == nil {
		return 0
	}
	iv, ok := sub.(*expvar.Int)
	if !ok {
		t.Fatalf("office_loop_run_claimed_total[%q] is not an Int", key)
	}
	return iv.Value()
}

// REQ-OFFICE-LOOP-LIVENESS-003: ClaimNextRun counts a successfully
// claimed run labelled by the claiming agent's workspace.
func TestClaimNextRun_IncrementsRunClaimedCounter(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("run-claimed-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	key := service.LoopMetricLabel("workspace", agent.WorkspaceID)
	before := runClaimedExpvarInt(t, key)

	req, err := svc.ClaimNextRun(ctx)
	if err != nil || req == nil {
		t.Fatalf("claim: req=%v err=%v", req, err)
	}

	after := runClaimedExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("run_claimed delta = %d, want 1", after-before)
	}
}

// ClaimNextRun with no queued work never increments the counter.
func TestClaimNextRun_EmptyQueueDoesNotIncrementRunClaimedCounter(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	key := service.LoopMetricLabel("workspace", "ws-1")
	before := runClaimedExpvarInt(t, key)

	req, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if req != nil {
		t.Fatalf("expected nil run, got %+v", req)
	}

	after := runClaimedExpvarInt(t, key)
	if after != before {
		t.Fatalf("run_claimed delta = %d, want 0", after-before)
	}
}
