package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type nilWorkflowRepository struct {
	taskrepository.WorkflowRepository
}

func (nilWorkflowRepository) GetWorkflow(context.Context, string) (*models.Workflow, error) {
	return nil, nil
}

func TestCreateTaskRejectsWorkflowOutsideRequestedWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	for _, workspace := range []*models.Workspace{
		{ID: "ws-a", Name: "A", OwnerID: "user-a"},
		{ID: "ws-b", Name: "B", OwnerID: "user-b"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-b", WorkspaceID: "ws-b", Name: "B workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	_, err := svc.CreateTask(ctxAs("user-a"), &CreateTaskRequest{
		WorkspaceID: "ws-a",
		WorkflowID:  "wf-b",
		Title:       "cross-workspace task",
	})
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("CreateTask error = %v, want workspace not found", err)
	}

	tasks, listErr := repo.ListTasks(ctx, "wf-b")
	if listErr != nil {
		t.Fatalf("ListTasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("persisted %d cross-workspace tasks, want none", len(tasks))
	}
}

func TestCreateTaskRejectsNonexistentWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "missing-workflow",
		Title:       "orphaned task",
	})
	if err == nil {
		t.Fatal("CreateTask succeeded with a nonexistent workflow")
	}

	tasks, listErr := repo.ListTasks(ctx, "missing-workflow")
	if listErr != nil {
		t.Fatalf("ListTasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("persisted %d tasks with a nonexistent workflow, want none", len(tasks))
	}
}

func TestCreateTaskRejectsNilWorkflowResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	svc.workflows = nilWorkflowRepository{WorkflowRepository: repo}

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "missing-workflow",
		Title:       "orphaned task",
	})
	if !errors.Is(err, ErrInvalidTaskWorkflow) {
		t.Fatalf("CreateTask error = %v, want invalid task workflow", err)
	}
}

func TestCreateTaskRejectsExplicitStepOutsideRequestedWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	for _, workflow := range []*models.Workflow{
		{ID: "wf-a", WorkspaceID: "ws-1", Name: "A workflow"},
		{ID: "wf-b", WorkspaceID: "ws-1", Name: "B workflow"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow(%s): %v", workflow.ID, err)
		}
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-b": {ID: "step-b", WorkflowID: "wf-b", Name: "B step"},
	}})

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-a",
		WorkflowStepID: "step-b",
		Title:          "cross-workflow step task",
	})
	if !errors.Is(err, ErrInvalidTaskWorkflow) {
		t.Fatalf("CreateTask error = %v, want invalid task workflow", err)
	}

	tasks, listErr := repo.ListTasks(ctx, "wf-a")
	if listErr != nil {
		t.Fatalf("ListTasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("persisted %d cross-workflow step tasks, want none", len(tasks))
	}
}

func TestCreateTaskRejectsNonexistentExplicitStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{}})

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "missing-step",
		Title:          "orphaned step task",
	})
	if !errors.Is(err, ErrInvalidTaskWorkflow) {
		t.Fatalf("CreateTask error = %v, want invalid task workflow", err)
	}

	tasks, listErr := repo.ListTasks(ctx, "wf-1")
	if listErr != nil {
		t.Fatalf("ListTasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("persisted %d tasks with a nonexistent explicit step, want none", len(tasks))
	}
}

func TestCreateTaskRejectsExplicitStepWhenValidationUnavailable(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	svc.SetWorkflowStepGetter(nil)

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "unvalidated step task",
	})
	if !errors.Is(err, ErrInvalidTaskWorkflow) {
		t.Fatalf("CreateTask error = %v, want invalid task workflow", err)
	}
}

func TestCreateTaskRejectsExplicitStepWithoutWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowStepID: "step-1",
		Title:          "orphaned step task",
		IsEphemeral:    true,
	})
	if !errors.Is(err, ErrInvalidTaskWorkflow) {
		t.Fatalf("CreateTask error = %v, want invalid task workflow", err)
	}
}

func TestCreateTaskAcceptsConsistentWorkflowAndExplicitStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-1": {ID: "step-1", WorkflowID: "wf-1", Name: "Step"},
	}})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "consistent task",
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.WorkspaceID != "ws-1" || task.WorkflowID != "wf-1" || task.WorkflowStepID != "step-1" {
		t.Fatalf("created task has inconsistent IDs: %+v", task)
	}
}

func TestCreateTaskAcceptsEphemeralTaskWithoutWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "quick chat",
		IsEphemeral: true,
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.WorkflowID != "" || task.WorkflowStepID != "" || !task.IsEphemeral {
		t.Fatalf("created ephemeral task has unexpected workflow fields: %+v", task)
	}
}

func TestListTasksExcludesLegacyRowsFromAnotherWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	for _, workspace := range []*models.Workspace{
		{ID: "ws-a", Name: "A", OwnerID: "user-a"},
		{ID: "ws-b", Name: "B", OwnerID: "user-b"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-b", WorkspaceID: "ws-b", Name: "B workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	for _, task := range []*models.Task{
		{
			ID: "valid-b", WorkspaceID: "ws-b", WorkflowID: "wf-b",
			Title: "valid", State: v1.TaskStateCreated, Priority: defaultPriority,
		},
		{
			ID: "legacy-a", WorkspaceID: "ws-a", WorkflowID: "wf-b",
			Title: "legacy inconsistent", State: v1.TaskStateCreated, Priority: defaultPriority,
		},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.ID, err)
		}
	}

	tasks, err := svc.ListTasks(ctxAs("user-b"), "wf-b")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "valid-b" {
		t.Fatalf("ListTasks returned %+v, want only valid-b", tasks)
	}
}

func TestListTasksRejectsNilWorkflowResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workflows = nilWorkflowRepository{WorkflowRepository: repo}

	if _, err := svc.ListTasks(context.Background(), "missing-workflow"); err == nil {
		t.Fatal("ListTasks succeeded with a nil workflow result")
	}
}

func TestDeleteWorkflowRejectsNilWorkflowResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:          "legacy-task",
		WorkspaceID: "ws-1",
		WorkflowID:  "missing-workflow",
		Title:       "Legacy task",
		State:       v1.TaskStateCreated,
		Priority:    defaultPriority,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	svc.workflows = nilWorkflowRepository{WorkflowRepository: repo}

	if err := svc.DeleteWorkflow(ctx, "missing-workflow"); err == nil {
		t.Fatal("DeleteWorkflow succeeded with a nil workflow result")
	}
}
