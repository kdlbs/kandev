package service_test

import (
	"context"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	"testing"
)

func TestMultipleOrchestratorsReceiveOnlyTheirOwnedTaskEvents(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.ExecSQL(t, `INSERT OR IGNORE INTO workspaces(id) VALUES('ws-1')`)
	for _, id := range []string{"coordinator-one", "coordinator-two"} {
		createTestAgent(t, svc, "ws-1", id)
		svc.ExecSQL(t, `INSERT INTO workspace_orchestrators(agent_id,workspace_id,role_id) VALUES(?,'ws-1','chief-of-staff')`, id)
	}
	insertTestTask(t, svc, "second-owned", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET state='REVIEW',metadata='{"orchestration_chief_id":"coordinator-two"}' WHERE id='second-owned'`)
	event := bus.NewEvent(events.TaskStateChanged, "test", map[string]string{"task_id": "second-owned"})
	for i := 0; i < 2; i++ {
		if err := eb.Publish(ctx, events.TaskStateChanged, event); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, run := range runs {
		if run.Reason != service.RunReasonWorkspaceTaskCallback {
			continue
		}
		if run.AgentProfileID != "coordinator-two" {
			t.Fatalf("wrong orchestrator woken: %s", run.AgentProfileID)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("expected one notification for owning orchestrator, got %d", count)
	}
}
