package service_test

// RR19-F3 follow-up: pins the durable operator-visible record
// (REQ-OFFICE-LAUNCH-SAFETY-003/REQ-OFFICE-BACKPRESSURE-003) that
// handleLaunchDeferred writes on the legacy direct-launch fallback path
// when the orchestrator's session ceiling defers a launch. This path had
// no test coverage before this change; its routed-dispatch sibling in
// internal/office/scheduler now writes the same record (see
// dispatch_routing_launch_deferred_event_test.go).

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/service"
)

func TestSchedulerTick_LaunchDeferredByCapacity_ParksAndRecordsDurableRunEvent(t *testing.T) {
	mock := &mockTaskStarter{err: service.ErrLaunchDeferredByCapacity}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID:                 "deferred-agent-1",
		WorkspaceID:        "ws-1",
		Name:               "deferred-worker",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES ('task-deferred-1', 'ws-1', 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-deferred-1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if mock.callCount() != 1 {
		t.Fatalf("expected 1 StartTask call, got %d", mock.callCount())
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	gotRun := runs[0]
	if gotRun.RoutingBlockedStatus == nil || string(*gotRun.RoutingBlockedStatus) != routing.StatusBlockedActionRequired {
		t.Fatalf("routing block = %v, want %s", gotRun.RoutingBlockedStatus, routing.StatusBlockedActionRequired)
	}

	events, err := svc.ListRunEventsForTest(ctx, gotRun.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	var found bool
	for _, e := range events {
		if string(e.EventType) != "adapter.invoke" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
			continue
		}
		if payload["phase"] == "deferred" && payload["reason"] == "session_ceiling" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %+v, want a durable adapter.invoke event with phase=deferred reason=session_ceiling", events)
	}
}
