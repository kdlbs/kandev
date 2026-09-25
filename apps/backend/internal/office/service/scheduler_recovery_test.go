package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
)

func TestOfficeRecoveryHandler_SkipsWithoutOfficeAdoption(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-office")
	insertTestTask(t, svc, "task-office-project", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "task-office-project")
	setTestTaskAssignee(t, svc, "task-office-project", "worker-office")

	handler := service.NewOfficeRecoveryHandler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("recovery tick: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("recovery ran without Office adoption: %#v", runs)
	}
}

func TestOfficeRecoveryHandlerActivatesAfterOfficeProjectAdoption(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-office")
	insertTestTask(t, svc, "task-office-project", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "task-office-project")
	setTestTaskAssignee(t, svc, "task-office-project", "worker-office")

	handler := service.NewOfficeRecoveryHandler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("initial recovery tick: %v", err)
	}

	svc.ExecSQL(t, `
		INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at)
		VALUES ('office-project', 'ws-1', 'Office Project', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("activated recovery tick: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Reason != service.RunReasonTaskAssigned {
		t.Fatalf("recovery runs = %#v, want one task_assigned run", runs)
	}
}

// TestOfficeRecoveryHandler_TaskAssignedNeverCarriesAgentActorType pins
// AC-OFFICE-ASSIGN-RATE-001.12 for recoverUnstartedTasks: its
// task_assigned payload is a bare {"task_id":...} with no actor_type
// field, so it can never fall into the assignment-wake-rate-limit's
// scope.
func TestOfficeRecoveryHandler_TaskAssignedNeverCarriesAgentActorType(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-office")
	insertTestTask(t, svc, "task-office-project", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "task-office-project")
	setTestTaskAssignee(t, svc, "task-office-project", "worker-office")
	svc.ExecSQL(t, `
		INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at)
		VALUES ('office-project', 'ws-1', 'Office Project', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)

	handler := service.NewOfficeRecoveryHandler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("recovery tick: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("recovery runs = %#v, want one task_assigned run", runs)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(runs[0].Payload), &payload); err != nil {
		t.Fatalf("decode run payload: %v", err)
	}
	if actorType, present := payload["actor_type"]; present && actorType == "agent" {
		t.Fatalf("recovery task_assigned payload carries actor_type=agent: %v", payload)
	}
}
