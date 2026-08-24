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

	// No session should have been launched.
	if mock.callCount() != 0 {
		t.Errorf("expected 0 StartTask calls, got %d", mock.callCount())
	}

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
