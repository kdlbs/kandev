package service_test

// TestProducer_EventSubscriber_TaskAssignedNeverCarriesAgentActorType and
// its siblings in internal/office/onboarding, internal/office/service
// (scheduler_recovery), and internal/orchestrator pin
// AC-OFFICE-ASSIGN-RATE-001.12: only one of the five task_assigned
// producers — the mutation-reactivity path in internal/office/scheduler —
// can ever carry actor_type: "agent", because RunContext is the only
// payload-building type with that field. This file's test covers
// event_subscribers.go's queueTaskAssignedRun; scheduler_recovery_test.go
// covers recoverUnstartedTasks in this same package.

import (
	"context"
	"encoding/json"
	"testing"
)

func TestProducer_EventSubscriber_TaskAssignedNeverCarriesAgentActorType(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-scope")
	insertTestTask(t, svc, "task-scope", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "task-scope")

	publishTaskAssigned(t, ctx, eb, "task-scope", "worker-scope", gen(0))

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	found := false
	for _, run := range runs {
		if run.AgentProfileID != "worker-scope" || run.Reason != "task_assigned" {
			continue
		}
		found = true
		var payload map[string]any
		if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
			t.Fatalf("decode run payload: %v", err)
		}
		if actorType, present := payload["actor_type"]; present && actorType == "agent" {
			t.Fatalf("event-subscriber task_assigned payload carries actor_type=agent, "+
				"which would put it in the assignment-wake-rate-limit's scope: %v", payload)
		}
	}
	if !found {
		t.Fatal("expected a task_assigned run for worker-scope, found none")
	}
}
