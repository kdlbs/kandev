package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkflowStepQueueLimitChange(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "queue-change-workspace", Name: "Queue change workspace", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "queue-change-workflow", WorkspaceID: "queue-change-workspace", Name: "Queue change workflow",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	step := &wfmodels.WorkflowStep{
		ID: "queue-change-review", WorkflowID: "queue-change-workflow", Name: "Review", Position: 0,
		WIPLimit: 1,
	}
	steps := newMockStepGetter()
	steps.steps[step.ID] = step

	createTask := func(id string, admitted bool, queued bool) {
		task := &models.Task{
			ID: id, WorkspaceID: "queue-change-workspace", WorkflowID: step.WorkflowID,
			WorkflowStepID: step.ID, Title: id, State: v1.TaskStateInProgress,
			WIPAdmitted: admitted,
		}
		if queued {
			task.QueuedForStepID = step.ID
			task.QueuedAt = &now
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", id, err)
		}
	}
	createTask("queue-change-occupant", true, false)
	createTask("queue-change-first", false, true)
	createTask("queue-change-second", false, true)

	promotions := 0
	store := newWorkflowStore(
		repo, steps, nil, func(context.Context, *models.Task, ...string) {}, testLogger(), &operationLedger{},
		func(context.Context, *models.Task) { promotions++ },
	)
	service := &Service{workflowStore: store}
	workflowStepUpdated := func() *bus.Event {
		return bus.NewEvent(events.WorkflowStepUpdated, "test", map[string]interface{}{
			"step": map[string]interface{}{"id": step.ID, "wip_limit": step.WIPLimit},
		})
	}

	// A lower limit never removes an admitted occupant or promotes work into
	// an already full destination.
	if err := service.handleWorkflowStepQueueUpdate(ctx, workflowStepUpdated()); err != nil {
		t.Fatalf("initial workflow step queue update: %v", err)
	}
	first, err := repo.GetTask(ctx, "queue-change-first")
	if err != nil {
		t.Fatalf("load first queued task: %v", err)
	}
	if first.WIPAdmitted {
		t.Fatal("first queued task was promoted while the destination was full")
	}

	step.WIPLimit = 2
	if err := service.handleWorkflowStepQueueUpdate(ctx, workflowStepUpdated()); err != nil {
		t.Fatalf("raise WIP limit: %v", err)
	}
	first, err = repo.GetTask(ctx, "queue-change-first")
	if err != nil {
		t.Fatalf("load promoted task: %v", err)
	}
	if !first.WIPAdmitted || first.QueuedForStepID != "" {
		t.Fatalf("first task after promotion = %+v", first)
	}
	if promotions != 1 {
		t.Fatalf("promotions after raising limit = %d, want 1", promotions)
	}

	step.WIPLimit = 1
	if err := service.handleWorkflowStepQueueUpdate(ctx, workflowStepUpdated()); err != nil {
		t.Fatalf("raised workflow step queue update: %v", err)
	}
	second, err := repo.GetTask(ctx, "queue-change-second")
	if err != nil {
		t.Fatalf("load second queued task: %v", err)
	}
	if second.WIPAdmitted {
		t.Fatal("lowering the limit displaced or admitted a queued task")
	}
	if promotions != 1 {
		t.Fatalf("promotions after lowering limit = %d, want 1", promotions)
	}

	step.WIPLimit = 0
	if err := service.handleWorkflowStepQueueUpdate(ctx, workflowStepUpdated()); err != nil {
		t.Fatalf("remove WIP limit: %v", err)
	}
	second, err = repo.GetTask(ctx, "queue-change-second")
	if err != nil {
		t.Fatalf("load second promoted task: %v", err)
	}
	if !second.WIPAdmitted || second.QueuedForStepID != "" {
		t.Fatalf("second task after unlimited promotion = %+v", second)
	}
	if promotions != 2 {
		t.Fatalf("promotions after removing limit = %d, want 2", promotions)
	}

	// Repeated update events are harmless after the queue is drained.
	if err := service.handleWorkflowStepQueueUpdate(ctx, workflowStepUpdated()); err != nil {
		t.Fatalf("unlimited workflow step queue update: %v", err)
	}
	if promotions != 2 {
		t.Fatalf("duplicate update promoted work again: %d", promotions)
	}
}
