package service_test

import (
	"context"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	"testing"
)

func TestWorkspaceChiefObservesReviewOnce(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "chief-board")
	svc.ExecSQL(t, `INSERT OR IGNORE INTO workspaces(id) VALUES('ws-1')`)
	svc.ExecSQL(t, `INSERT INTO office_workspace_chief(workspace_id,agent_profile_id) VALUES('ws-1','chief-board')`)
	insertTestTask(t, svc, "adopted", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET state='REVIEW',metadata='{"orchestration_chief_id":"chief-board"}' WHERE id='adopted'`)
	event := bus.NewEvent(events.TaskStateChanged, "test", map[string]string{"task_id": "adopted"})
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
		if run.AgentProfileID == "chief-board" && run.Reason == service.RunReasonTaskChildrenCompleted {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one review notification, got %d", count)
	}
}
