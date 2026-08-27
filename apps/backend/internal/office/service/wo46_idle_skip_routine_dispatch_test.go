package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// TestIdleSkip_RoutineDispatchNoTasks_Skipped is the WO-46 regression test.
// Production routine dispatch (internal/office/routines) queues runs with
// reason "routine_dispatch", not RunReasonHeartbeat — checkIdleSkip must
// recognize that reason too, or the idle-skip gate is unreachable in
// production even though the existing RunReasonHeartbeat-driven tests pass.
func TestIdleSkip_RoutineDispatchNoTasks_Skipped(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-worker-routine-dispatch",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Worker defaults to skip_idle_runs=true, no tasks assigned.

	if err := svc.QueueRun(ctx, agent.ID, shared.RunReasonRoutineDispatch, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	// mock.callCount() == 0 is not asserted here: a taskless run never
	// reaches StartTask regardless of whether the idle-skip gate fired
	// correctly, so it can't discriminate a working gate from a broken one
	// (WO-46 Review round 1, S1). The activity-entry and queue-drained
	// checks below are what actually prove the skip happened.

	// Queue should be empty (run was finished).
	next, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim after tick: %v", err)
	}
	if next != nil {
		t.Error("expected queue to be empty after idle skip")
	}

	// Activity log should have a run_idle_skipped entry.
	entries, err := svc.ListActivity(ctx, "ws-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "run_idle_skipped" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected run_idle_skipped activity entry for reason=routine_dispatch")
	}
}

// TestIdleSkip_RoutineDispatch_CoordinatorNotSkipped is the complementary
// case to TestIdleSkip_RoutineDispatchNoTasks_Skipped (PR #2973 review
// round 1, github-actions suggestion): a coordinator (CEO role) defaults to
// SkipIdleRuns=false because its heartbeat purpose is self-directed and
// does not require a directly assigned task, so a routine_dispatch fire for
// one must never be idle-skipped. checkIdleSkip's guard order happens to
// check the reason before SkipIdleRuns, so this passes today, but nothing
// pinned the coordinator side of the contract — a future reorder of the
// guards in checkIdleSkip would silently break it without this test.
func TestIdleSkip_RoutineDispatch_CoordinatorNotSkipped(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-2",
		Name:               "coordinator-no-skip",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if agent.SkipIdleRuns {
		t.Fatalf("CEO role should default to SkipIdleRuns=false")
	}

	if err := svc.QueueRun(ctx, agent.ID, shared.RunReasonRoutineDispatch, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	entries, err := svc.ListActivity(ctx, "ws-2", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "run_idle_skipped" {
			t.Error("coordinator run must not be idle-skipped when SkipIdleRuns=false")
		}
	}
}
