package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

type circuitSuspension struct{ until time.Time }

func (e circuitSuspension) Error() string                    { return "resource suspended" }
func (e circuitSuspension) CircuitSuspendedUntil() time.Time { return e.until }

func TestSchedulerDefersSuspendedCircuitWithoutConsumingRetry(t *testing.T) {
	deadline := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	starter := &mockTaskStarter{err: circuitSuspension{until: deadline}}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: starter})
	ctx := context.Background()
	agent := &models.AgentInstance{ID: "circuit-agent", WorkspaceID: "ws-1", Name: "worker", Role: models.AgentRoleWorker, Status: models.AgentStatusIdle, ExecutorPreference: `{"type":"worktree"}`}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatal(err)
	}
	insertTestTask(t, svc, "circuit-task", "ws-1")
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"circuit-task"}`, ""); err != nil {
		t.Fatal(err)
	}
	service.RunSchedulerTick(svc, ctx)
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs: %v, %v", runs, err)
	}
	run := runs[0]
	if run.Status != service.RunStatusQueued || run.RetryCount != 0 || run.ScheduledRetryAt == nil || !run.ScheduledRetryAt.Equal(deadline) {
		t.Fatalf("suspended run: status=%s retries=%d retry_at=%v", run.Status, run.RetryCount, run.ScheduledRetryAt)
	}
}

func TestSchedulerParksHeldCircuitWithoutConsumingRetry(t *testing.T) {
	starter := &mockTaskStarter{err: circuitSuspension{}}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: starter})
	ctx := context.Background()
	agent := &models.AgentInstance{ID: "held-agent", WorkspaceID: "ws-1", Name: "worker", Role: models.AgentRoleWorker, Status: models.AgentStatusIdle, ExecutorPreference: `{"type":"worktree"}`}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatal(err)
	}
	insertTestTask(t, svc, "held-task", "ws-1")
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"held-task"}`, ""); err != nil {
		t.Fatal(err)
	}
	service.RunSchedulerTick(svc, ctx)
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs: %v, %v", runs, err)
	}
	run := runs[0]
	if run.Status != service.RunStatusQueued || run.RoutingBlockedStatus == nil || string(*run.RoutingBlockedStatus) != "blocked_provider_action_required" || run.RetryCount != 0 || run.ScheduledRetryAt != nil {
		t.Fatalf("held run: status=%s retries=%d retry_at=%v", run.Status, run.RetryCount, run.ScheduledRetryAt)
	}
}
