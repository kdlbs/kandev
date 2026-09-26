package service_test

// TestOfficeRecoveryHandler_StepEligibility is the ISSUE-5 regression for
// recoverUnstartedTasks (scheduler_recovery.go): the recovery sweep must not
// re-queue a task_assigned wake for a task sitting on a workflow step with no
// auto_start_agent on_enter action, for the same reason as
// TestQueueTaskAssignedRun_StepEligibility in this package.
import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestOfficeRecoveryHandler_StepEligibility(t *testing.T) {
	const stepBacklog = "step-backlog-recovery"
	const stepWork = "step-work-recovery"

	svc := newTestService(t)
	ctx := context.Background()
	svc.SetWorkflowStepGetter(&fakeAssignmentStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepBacklog: backlogWorkflowStep(stepBacklog),
		stepWork:    autoStartWorkflowStep(stepWork),
	}})

	createTestAgent(t, svc, "ws-1", "worker-office")
	for i := 1; i <= 5; i++ {
		taskID := fmt.Sprintf("task-office-backlog-%d", i)
		insertTestTask(t, svc, taskID, "ws-1")
		svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project', workflow_step_id = ? WHERE id = ?`,
			stepBacklog, taskID)
		setTestTaskAssignee(t, svc, taskID, "worker-office")
	}
	insertTestTask(t, svc, "task-office-work", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project', workflow_step_id = ? WHERE id = ?`,
		stepWork, "task-office-work")
	setTestTaskAssignee(t, svc, "task-office-work", "worker-office")
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
		t.Fatalf("recovery sweep queued %d runs, want the eligible task after five ineligible candidates: %#v", len(runs), runs)
	}
	if runs[0].AgentProfileID != "worker-office" {
		t.Fatalf("recovery sweep queued run for agent %q, want worker-office", runs[0].AgentProfileID)
	}
}
