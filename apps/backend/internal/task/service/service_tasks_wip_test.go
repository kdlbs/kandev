package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fixedStartStepResolver struct {
	stepID string
}

func (r fixedStartStepResolver) ResolveStartStep(context.Context, string) (string, error) {
	return r.stepID, nil
}

func (r fixedStartStepResolver) ResolveFirstStep(context.Context, string) (string, error) {
	return r.stepID, nil
}

func (r fixedStartStepResolver) ResolveAutoStartStep(context.Context, string) (string, error) {
	return r.stepID, nil
}

func TestCreateTask_QueuesFullWIPStepWithoutFeeder(t *testing.T) {
	svc, events, repo := createTestService(t)
	ctx := context.Background()
	seedWIPWorkflow(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"review-step": {ID: "review-step", WorkflowID: "wip-workflow", Name: "Review", WIPLimit: 1},
	}})
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "wip-occupant", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "review-step", Title: "Occupant", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}

	queuedResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow", WorkflowStepID: "review-step",
		Title: "Rejected", Description: "must not persist",
	})
	queued := queuedResult.Task
	if err != nil {
		t.Fatalf("error=%v, want queued success", err)
	}
	if _, err := repo.GetTask(ctx, "wip-occupant"); err != nil {
		t.Fatalf("occupant disappeared: %v", err)
	}
	tasks, err := repo.ListTasksByWorkflowStep(ctx, "review-step")
	if err != nil {
		t.Fatalf("list step tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("step task count=%d, want 2", len(tasks))
	}
	if queued.WIPAdmitted || queued.QueuedForStepID != "review-step" {
		t.Fatalf("queued task admission=%v target=%q", queued.WIPAdmitted, queued.QueuedForStepID)
	}
	if len(events.GetPublishedEvents()) != 1 {
		t.Fatalf("published events=%d, want task.created", len(events.GetPublishedEvents()))
	}
}

func TestCreateTask_ResolvedStartStepUsesWIPAdmission(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedWIPWorkflow(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"review-step": {ID: "review-step", WorkflowID: "wip-workflow", Name: "Review", WIPLimit: 1},
	}})
	svc.SetStartStepResolver(fixedStartStepResolver{stepID: "review-step"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "wip-occupant-resolved", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "review-step", Title: "Occupant", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}

	queuedResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		Title: "Rejected resolved start", Description: "must not persist",
	})
	queued := queuedResult.Task
	if err != nil {
		t.Fatalf("error=%v, want queued success", err)
	}
	if queued.QueuedForStepID != "review-step" || queued.WIPAdmitted {
		t.Fatalf("queued task admission=%v target=%q", queued.WIPAdmitted, queued.QueuedForStepID)
	}
}

func TestCreateTask_UnlimitedWIPStepPreservesCreation(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedWIPWorkflow(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"unlimited-step": {ID: "unlimited-step", WorkflowID: "wip-workflow", Name: "Unlimited"},
	}})

	for i := 0; i < 3; i++ {
		if _, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow", WorkflowStepID: "unlimited-step",
			Title: "Unlimited task", Description: "allowed",
		}); err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
	}
	occupants, err := repo.CountTasksByWorkflowStep(ctx, "unlimited-step")
	if err != nil {
		t.Fatalf("count occupants: %v", err)
	}
	if occupants != 3 {
		t.Fatalf("occupants=%d, want 3", occupants)
	}
}

func TestCreateTask_PullsUnstartedFeederTaskIntoAvailableWIPStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedWIPWorkflow(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"waiting-step": {ID: "waiting-step", WorkflowID: "wip-workflow", Name: "Waiting"},
		"review-step":  {ID: "review-step", WorkflowID: "wip-workflow", Name: "Review", WIPLimit: 2, PullFromStepID: "waiting-step"},
	}})

	createdResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow", WorkflowStepID: "waiting-step",
		Title: "Unstarted review task",
	})
	created := createdResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.WorkflowStepID != "review-step" {
		t.Fatalf("returned workflow step = %q, want review-step", created.WorkflowStepID)
	}

	stored, err := repo.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkflowStepID != "review-step" {
		t.Fatalf("workflow step = %q, want review-step", stored.WorkflowStepID)
	}
}

func TestCreateTask_FeederTaskStaysWhenWIPStepFull(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedWIPWorkflow(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"waiting-step": {ID: "waiting-step", WorkflowID: "wip-workflow", Name: "Waiting"},
		"review-step":  {ID: "review-step", WorkflowID: "wip-workflow", Name: "Review", WIPLimit: 1, PullFromStepID: "waiting-step"},
	}})
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "occupant", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "review-step", WIPAdmitted: true, State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}

	createdResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow", WorkflowStepID: "waiting-step",
		Title: "Should stay in feeder",
	})
	created := createdResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.WorkflowStepID != "waiting-step" {
		t.Fatalf("step = %q, want waiting-step (WIP full, must not promote)", created.WorkflowStepID)
	}
}

func seedWIPWorkflow(t *testing.T, ctx context.Context, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
}) {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "wip-workspace", Name: "WIP Workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wip-workflow", WorkspaceID: "wip-workspace", Name: "WIP Workflow"}); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
}
