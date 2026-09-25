package service_test

// TestOfficeRecoveryHandler_StepEligibility is the ISSUE-5 regression for
// recoverUnstartedTasks (scheduler_recovery.go): the recovery sweep must not
// re-queue a task_assigned wake for a task sitting on a workflow step with no
// auto_start_agent on_enter action, for the same reason as
// TestQueueTaskAssignedRun_StepEligibility in this package.
import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestOfficeRecoveryHandler_StepEligibility(t *testing.T) {
	const stepBacklog = "step-backlog-recovery"

	svc := newTestService(t)
	ctx := context.Background()
	svc.SetWorkflowStepGetter(&fakeAssignmentStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepBacklog: backlogWorkflowStep(stepBacklog),
	}})

	createTestAgent(t, svc, "ws-1", "worker-office")
	insertTestTask(t, svc, "task-office-project", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project', workflow_step_id = ? WHERE id = ?`,
		stepBacklog, "task-office-project")
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
	if len(runs) != 0 {
		t.Fatalf("recovery sweep queued %d runs for a backlog-step task, want 0: %#v", len(runs), runs)
	}
}
