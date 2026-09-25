package service_test

// Covers the queueRunInline fallback in run.go (no runs service wired): the
// models.Run{} literal it builds must stamp PriorityClass explicitly. The
// Go zero value for models.PriorityClass is PriorityClassHuman (0), the
// highest claim-order preference, so an omitted field would silently
// promote every one of these runs (AC-OFFICE-BACKPRESSURE-001.1/.3). See
// the matching scheduler-package coverage in
// office/scheduler/run_priority_class_test.go for the QueueRun/QueueRunCtx
// counterpart of this same bug.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func TestQueueRun_InlineFallbackStampsEventPriorityClass(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	reqs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 run, got %d", len(reqs))
	}
	if reqs[0].PriorityClass != models.PriorityClassEvent {
		t.Errorf("priority_class = %d, want %d (PriorityClassEvent)", reqs[0].PriorityClass, models.PriorityClassEvent)
	}
}
