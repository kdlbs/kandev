package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// seedReorderTestStep inserts a workflow_steps row directly (same shape as
// the repository-level reorder tests) and wires a fakeWorkflowStepGetter so
// Service.ReorderStepTasks can resolve it without a full workflow service.
func seedReorderTestStep(t *testing.T, svc *Service, repo *sqliterepo.Repository, stepID, workflowID string, stepPosition int) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.DB().Exec(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		stepID, workflowID, stepID, stepPosition, now, now); err != nil {
		t.Fatalf("insert step %s: %v", stepID, err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepID: {ID: stepID, WorkflowID: workflowID, Name: stepID, Position: stepPosition},
	}})
}

func mustCreateReorderServiceTask(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, id, workspaceID, workflowID, stepID string) {
	t.Helper()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: id, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: stepID, Title: id,
	}); err != nil {
		t.Fatalf("CreateTask(%s): %v", id, err)
	}
}

func TestService_ReorderStepTasksCommitsAndPublishesEvent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-svc", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-svc", WorkspaceID: "ws-reorder-svc", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-svc", "wf-reorder-svc", 0)
	mustCreateReorderServiceTask(t, ctx, repo, "svc-task-a", "ws-reorder-svc", "wf-reorder-svc", "step-reorder-svc")
	mustCreateReorderServiceTask(t, ctx, repo, "svc-task-b", "ws-reorder-svc", "wf-reorder-svc", "step-reorder-svc")
	eventBus.ClearEvents()

	result, err := svc.ReorderStepTasks(ctx, "step-reorder-svc", "admitted", []string{"svc-task-b", "svc-task-a"})
	if err != nil {
		t.Fatalf("ReorderStepTasks: %v", err)
	}
	if result.WorkflowStepID != "step-reorder-svc" || result.Revision != 1 {
		t.Fatalf("result = %+v, want step-reorder-svc/revision 1", result)
	}
	if len(result.Tasks) != 2 || result.Tasks[0].ID != "svc-task-b" || result.Tasks[1].ID != "svc-task-a" {
		t.Fatalf("result.Tasks = %+v, want [svc-task-b svc-task-a]", result.Tasks)
	}

	found := false
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TaskReordered {
			continue
		}
		found = true
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("task.reordered payload is not a map: %#v", event.Data)
		}
		if data["workflow_step_id"] != "step-reorder-svc" {
			t.Fatalf("payload workflow_step_id = %v, want step-reorder-svc", data["workflow_step_id"])
		}
		if data["band"] != "admitted" {
			t.Fatalf("payload band = %v, want admitted", data["band"])
		}
		if data["workspace_id"] != "ws-reorder-svc" {
			t.Fatalf("payload workspace_id = %v, want ws-reorder-svc", data["workspace_id"])
		}
	}
	if !found {
		t.Fatal("no task.reordered event published")
	}
}

func TestService_ReorderStepTasksStepChangedCarriesAuthoritativeOrder(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-conflict", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-conflict", WorkspaceID: "ws-reorder-conflict", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-conflict", "wf-reorder-conflict", 0)
	mustCreateReorderServiceTask(t, ctx, repo, "conflict-a", "ws-reorder-conflict", "wf-reorder-conflict", "step-reorder-conflict")
	mustCreateReorderServiceTask(t, ctx, repo, "conflict-b", "ws-reorder-conflict", "wf-reorder-conflict", "step-reorder-conflict")
	eventBus.ClearEvents()

	// Submitted set is missing conflict-b: the classic membership-drift race.
	result, err := svc.ReorderStepTasks(ctx, "step-reorder-conflict", "admitted", []string{"conflict-a"})
	if !errors.Is(err, repoerrors.ErrStepChanged) {
		t.Fatalf("err = %v, want ErrStepChanged", err)
	}
	if result == nil || len(result.Tasks) != 2 {
		t.Fatalf("result = %+v, want the authoritative 2-task order", result)
	}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskReordered {
			t.Fatal("task.reordered must not publish on a rejected reorder")
		}
	}
}

func TestService_ReorderStepTasksInvalidRequestReturnsNilResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-invalid", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-invalid", WorkspaceID: "ws-reorder-invalid", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-invalid", "wf-reorder-invalid", 0)

	result, err := svc.ReorderStepTasks(ctx, "step-reorder-invalid", "not-a-band", []string{"whatever"})
	if !errors.Is(err, repoerrors.ErrInvalidReorder) {
		t.Fatalf("err = %v, want ErrInvalidReorder", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil on a malformed request", result)
	}
}
