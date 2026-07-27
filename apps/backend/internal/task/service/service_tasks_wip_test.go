package service

import (
	"context"
	"errors"
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

func TestCreateTask_RejectsFullWIPStepBeforePersistence(t *testing.T) {
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

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow", WorkflowStepID: "review-step",
		Title: "Rejected", Description: "must not persist",
	})
	if err == nil || !errors.Is(err, wfmodels.ErrWIPLimitExceeded) {
		t.Fatalf("error=%v, want typed WIP limit rejection", err)
	}
	if _, err := repo.GetTask(ctx, "wip-occupant"); err != nil {
		t.Fatalf("occupant disappeared: %v", err)
	}
	tasks, err := repo.ListTasksByWorkflowStep(ctx, "review-step")
	if err != nil {
		t.Fatalf("list step tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("step task count=%d, want 1", len(tasks))
	}
	if len(events.GetPublishedEvents()) != 0 {
		t.Fatalf("published events=%d, want none", len(events.GetPublishedEvents()))
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

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		Title: "Rejected resolved start", Description: "must not persist",
	})
	if err == nil || !errors.Is(err, wfmodels.ErrWIPLimitExceeded) {
		t.Fatalf("error=%v, want typed WIP limit rejection", err)
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
