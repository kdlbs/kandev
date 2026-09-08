package service_test

import (
	"context"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func terminalShapeExpvarInt(t *testing.T, key string) int64 {
	t.Helper()
	v := expvar.Get("office_loop_terminal_total")
	if v == nil {
		t.Fatal("expvar map office_loop_terminal_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatal("office_loop_terminal_total is not a Map")
	}
	sub := m.Get(key)
	if sub == nil {
		return 0
	}
	iv, ok := sub.(*expvar.Int)
	if !ok {
		t.Fatalf("office_loop_terminal_total[%q] is not an Int", key)
	}
	return iv.Value()
}

// REQ-OFFICE-LOOP-LIVENESS-005: FinishRun classifies the just-finished
// run and increments office_loop_terminal_total by shape. A run
// finished with outcome=processed and no session id classifies as
// silent_success (activation is published at repo construction in
// these in-memory test DBs, so the run's requested_at is after it).
func TestFinishRun_RecordsSilentSuccessTerminalShape(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("terminal-shape-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: run=%v err=%v", run, err)
	}

	key := service.LoopMetricLabel("workspace", agent.WorkspaceID, "shape", string(service.ShapeSilentSuccess))
	before := terminalShapeExpvarInt(t, key)

	if err := svc.FinishRun(ctx, run.ID, service.RunOutcomeProcessed); err != nil {
		t.Fatalf("finish: %v", err)
	}

	after := terminalShapeExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("silent_success delta = %d, want 1", after-before)
	}
}

// A failed run with no session id classifies as unlaunched_failed.
func TestFailRun_RecordsUnlaunchedFailedTerminalShape(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("terminal-shape-fail-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: run=%v err=%v", run, err)
	}

	key := service.LoopMetricLabel("workspace", agent.WorkspaceID, "shape", string(service.ShapeUnlaunchedFailed))
	before := terminalShapeExpvarInt(t, key)

	if err := svc.FailRun(ctx, run.ID); err != nil {
		t.Fatalf("fail: %v", err)
	}

	after := terminalShapeExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("unlaunched_failed delta = %d, want 1", after-before)
	}
}
