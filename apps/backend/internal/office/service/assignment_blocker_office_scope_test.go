package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-OFFICE-SCHEDULER-002.9
func TestOfficeAssignmentBlockerAdmissionKanban(t *testing.T) {
	f := newAssignmentBlockerFixture(t)
	ctx := context.Background()
	blockerMust(t, f.tasks.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "kanban", WorkspaceID: "assignment-ws", Name: "Kanban"}))
	blockerMust(t, f.workflows.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "kanban-work", WorkflowID: "kanban", Name: "Work", Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}}))
	blockerMust(t, f.tasks.CreateTask(ctx, &taskmodels.Task{ID: "kanban-task", WorkspaceID: "assignment-ws", WorkflowID: "kanban", WorkflowStepID: "kanban-work", Title: "Manual task", State: v1.TaskStateTODO, Priority: "medium", AssigneeAgentProfileID: "assignment-worker"}))
	f.assignment(t, events.TaskCreated, "kanban-task", "assignment-worker")
	if runs := f.taskRuns(t, "kanban-task", service.RunReasonTaskAssigned); len(runs) != 0 {
		t.Fatalf("Kanban assignment admitted %d Office runs", len(runs))
	}
	f.task(t, "office-task", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	f.assignment(t, events.TaskCreated, "office-task", "assignment-worker")
	if runs := f.taskRuns(t, "office-task", service.RunReasonTaskAssigned); len(runs) != 1 {
		t.Fatalf("Office positive control got %d runs", len(runs))
	}
}
