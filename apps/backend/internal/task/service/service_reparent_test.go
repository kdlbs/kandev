package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// reparentFixture spins up a service with a workspace + workflow and returns
// a helper that creates root tasks, so the nesting tests stay terse.
func reparentFixture(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository, func(title string) *models.Task) {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	create := func(title string) *models.Task {
		t.Helper()
		task, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf-1",
			WorkflowStepID: "step-1",
			Title:          title,
		})
		if err != nil {
			t.Fatalf("create task %q: %v", title, err)
		}
		return task
	}
	return svc, eventBus, repo, create
}

func strptr(s string) *string { return &s }

func TestService_UpdateTask_NestsUnderParent(t *testing.T) {
	svc, eventBus, _, create := reparentFixture(t)
	ctx := context.Background()
	parent := create("Parent")
	child := create("Child")
	eventBus.ClearEvents()

	updated, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(parent.ID)})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.ParentID != parent.ID {
		t.Errorf("in-memory ParentID = %q, want %q", updated.ParentID, parent.ID)
	}

	got, err := svc.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Errorf("persisted ParentID = %q, want %q", got.ParentID, parent.ID)
	}

	events := eventBus.GetPublishedEvents()
	if len(events) == 0 {
		t.Fatalf("expected a task.updated event, got none")
	}
	last := events[len(events)-1]
	if last.Type != "task.updated" {
		t.Errorf("event type = %q, want task.updated", last.Type)
	}
	data, ok := last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T", last.Data)
	}
	if data["parent_id"] != parent.ID {
		t.Errorf("event parent_id = %v, want %q", data["parent_id"], parent.ID)
	}
}

func TestService_UpdateTask_UnnestsWhenParentCleared(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	parent := create("Parent")
	child := create("Child")
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(parent.ID)}); err != nil {
		t.Fatalf("nest: %v", err)
	}

	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr("")}); err != nil {
		t.Fatalf("unnest: %v", err)
	}

	got, err := svc.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ParentID != "" {
		t.Errorf("persisted ParentID = %q, want empty", got.ParentID)
	}
}

func TestService_UpdateTask_LeavesParentUnchangedWhenNil(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	parent := create("Parent")
	child := create("Child")
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(parent.ID)}); err != nil {
		t.Fatalf("nest: %v", err)
	}

	// A title-only update must not touch the parent relationship.
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{Title: strptr("Renamed")}); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := svc.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Errorf("ParentID = %q, want preserved %q", got.ParentID, parent.ID)
	}
}

func TestService_UpdateTask_RejectsSelfParent(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	task := create("Task")

	_, err := svc.UpdateTask(ctx, task.ID, &UpdateTaskRequest{ParentID: strptr(task.ID)})
	if err == nil || !strings.Contains(err.Error(), "own parent") {
		t.Fatalf("expected self-parent error, got %v", err)
	}
}

func TestService_UpdateTask_RejectsMissingParent(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	task := create("Task")

	_, err := svc.UpdateTask(ctx, task.ID, &UpdateTaskRequest{ParentID: strptr("does-not-exist")})
	if err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("expected missing-parent error, got %v", err)
	}
}

func TestService_UpdateTask_RejectsCycle(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	a := create("A")
	b := create("B")
	// B nested under A.
	if _, err := svc.UpdateTask(ctx, b.ID, &UpdateTaskRequest{ParentID: strptr(a.ID)}); err != nil {
		t.Fatalf("nest B under A: %v", err)
	}
	// Nesting A under B would create A -> B -> A.
	_, err := svc.UpdateTask(ctx, a.ID, &UpdateTaskRequest{ParentID: strptr(b.ID)})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestService_UpdateTask_RejectsCrossWorkspaceParent(t *testing.T) {
	svc, _, repo, create := reparentFixture(t)
	ctx := context.Background()
	child := create("Child")

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other"}); err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	other := &models.Task{ID: "other-ws-parent", WorkspaceID: "ws-2", WorkflowID: "wf-1", Title: "Other"}
	if err := repo.CreateTask(ctx, other); err != nil {
		t.Fatalf("create cross-ws parent: %v", err)
	}

	_, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(other.ID)})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("expected cross-workspace error, got %v", err)
	}
}
