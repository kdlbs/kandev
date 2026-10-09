package service_test

// TestQueueTaskAssignedRun_StepEligibility and
// TestOfficeRecoveryHandler_StepEligibility are the ISSUE-5 regressions for
// this package's two task_assigned producers: queueTaskAssignedRun
// (event_subscribers.go, reached via task.updated) and recoverUnstartedTasks
// (scheduler_recovery.go). Both must skip queuing a wake when the task's
// current workflow step carries no auto_start_agent on_enter action —
// otherwise the agent runs outside the workflow on a step like Backlog,
// calls step_complete_kandev (accepted:true, nothing advances), and then
// runs again when the card later reaches an eligible step.

import (
	"context"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeAssignmentStepGetter is a minimal shared.AssignmentStepGetter test
// double: a static step map, or a fixed error.
type fakeAssignmentStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
	err   error
}

func (f *fakeAssignmentStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.steps[stepID], nil
}

func autoStartWorkflowStep(id string) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID: id,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
}

func backlogWorkflowStep(id string) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{ID: id, Events: wfmodels.StepEvents{}}
}

func TestQueueTaskAssignedRun_StepEligibility(t *testing.T) {
	const stepBacklog = "step-backlog"
	const stepAutoStart = "step-work"

	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetWorkflowStepGetter(&fakeAssignmentStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepBacklog:   backlogWorkflowStep(stepBacklog),
		stepAutoStart: autoStartWorkflowStep(stepAutoStart),
	}})

	createTestAgent(t, svc, "ws-1", "worker-backlog")
	insertTestTask(t, svc, "task-backlog", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project', workflow_step_id = ? WHERE id = ?`,
		stepBacklog, "task-backlog")

	createTestAgent(t, svc, "ws-1", "worker-work")
	insertTestTask(t, svc, "task-work", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project', workflow_step_id = ? WHERE id = ?`,
		stepAutoStart, "task-work")

	publishTaskAssigned(t, ctx, eb, "task-backlog", "worker-backlog", gen(0))
	publishTaskAssigned(t, ctx, eb, "task-work", "worker-work", gen(0))

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var backlogRuns, workRuns int
	for _, run := range runs {
		switch run.AgentProfileID {
		case "worker-backlog":
			backlogRuns++
		case "worker-work":
			workRuns++
		}
	}
	if backlogRuns != 0 {
		t.Fatalf("backlog-step task queued %d task_assigned runs, want 0", backlogRuns)
	}
	if workRuns != 1 {
		t.Fatalf("auto-start-step task queued %d task_assigned runs, want 1", workRuns)
	}
}
