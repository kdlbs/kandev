package service

import (
	"context"
	"errors"
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
	svc, eventBus, _, create := reparentFixture(t)
	ctx := context.Background()
	parent := create("Parent")
	child := create("Child")
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(parent.ID)}); err != nil {
		t.Fatalf("nest: %v", err)
	}
	eventBus.ClearEvents()

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

	// The un-nest event must carry an explicit parent_id: nil so clients can
	// distinguish "parent removed" from "parent unchanged".
	events := eventBus.GetPublishedEvents()
	if len(events) == 0 {
		t.Fatalf("expected a task.updated event, got none")
	}
	last := events[len(events)-1]
	data, ok := last.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T", last.Data)
	}
	val, present := data["parent_id"]
	if !present {
		t.Errorf("un-nest event is missing parent_id key")
	}
	if val != nil {
		t.Errorf("un-nest event parent_id = %#v, want nil", val)
	}
}

func TestService_UpdateTask_RejectsArchivedParent(t *testing.T) {
	svc, _, repo, create := reparentFixture(t)
	ctx := context.Background()
	parent := create("Parent")
	child := create("Child")
	if err := svc.ArchiveTask(ctx, parent.ID); err != nil {
		t.Fatalf("archive parent: %v", err)
	}
	_ = repo

	_, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(parent.ID)})
	if err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("expected archived-parent error, got %v", err)
	}
	if !errors.Is(err, ErrInvalidParent) {
		t.Errorf("archived-parent error should wrap ErrInvalidParent, got %v", err)
	}
}

func TestService_UpdateTask_ValidationErrorsWrapErrInvalidParent(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	a := create("A")
	b := create("B")
	if _, err := svc.UpdateTask(ctx, b.ID, &UpdateTaskRequest{ParentID: strptr(a.ID)}); err != nil {
		t.Fatalf("nest B under A: %v", err)
	}

	cases := map[string]string{
		"self":     a.ID,
		"cycle":    b.ID, // A under B would cycle
		"nonexist": "missing-task",
	}
	for name, parentID := range cases {
		_, err := svc.UpdateTask(ctx, a.ID, &UpdateTaskRequest{ParentID: strptr(parentID)})
		if err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		}
		if !errors.Is(err, ErrInvalidParent) {
			t.Errorf("%s: error should wrap ErrInvalidParent so it maps to HTTP 400, got %v", name, err)
		}
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

func TestService_UpdateTask_RejectsNestingUnderSubtask(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	root := create("Root")
	child := create("Child")
	other := create("Other")
	// child nested under root — child is now a subtask.
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(root.ID)}); err != nil {
		t.Fatalf("nest child under root: %v", err)
	}

	// Nesting other under child would create root -> child -> other (depth 2),
	// which the create path also forbids for kanban tasks.
	_, err := svc.UpdateTask(ctx, other.ID, &UpdateTaskRequest{ParentID: strptr(child.ID)})
	if err == nil {
		t.Fatalf("expected subtask-depth error, got nil")
	}
	if !errors.Is(err, ErrInvalidParent) {
		t.Errorf("depth error should wrap ErrInvalidParent (maps to HTTP 400), got %v", err)
	}
	if !errors.Is(err, ErrSubtaskDepthExceeded) {
		t.Errorf("depth error should wrap ErrSubtaskDepthExceeded, got %v", err)
	}
}

func TestService_UpdateTask_RejectsMovingTaskWithChildren(t *testing.T) {
	svc, _, _, create := reparentFixture(t)
	ctx := context.Background()
	root := create("Root")
	child := create("Child")
	target := create("Target")
	// child nested under root — root now has a child.
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(root.ID)}); err != nil {
		t.Fatalf("nest child under root: %v", err)
	}

	// Nesting root (which has a child) under target would push child to depth 2.
	_, err := svc.UpdateTask(ctx, root.ID, &UpdateTaskRequest{ParentID: strptr(target.ID)})
	if err == nil {
		t.Fatalf("expected subtask-depth error, got nil")
	}
	if !errors.Is(err, ErrInvalidParent) {
		t.Errorf("depth error should wrap ErrInvalidParent, got %v", err)
	}
	if !errors.Is(err, ErrSubtaskDepthExceeded) {
		t.Errorf("depth error should wrap ErrSubtaskDepthExceeded, got %v", err)
	}
}

func TestService_UpdateTask_AllowsDeepOfficeNesting(t *testing.T) {
	svc, _, repo, _ := reparentFixture(t)
	ctx := context.Background()
	// Office task trees intentionally allow arbitrary depth, so the one-level
	// guard must not fire when the endpoints are Office tasks. A non-empty
	// project_id is what the repository projection uses to mark a task
	// IsFromOffice.
	grandparent := &models.Task{ID: "o-gp", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "GP", ProjectID: "proj-1"}
	parent := &models.Task{ID: "o-parent", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "P", ParentID: "o-gp", ProjectID: "proj-1"}
	child := &models.Task{ID: "o-child", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "C", ProjectID: "proj-1"}
	for _, tk := range []*models.Task{grandparent, parent, child} {
		if err := repo.CreateTask(ctx, tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	// parent is already a subtask; nesting child under it is depth 2, which is
	// allowed for Office tasks.
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: strptr(parent.ID)}); err != nil {
		t.Fatalf("office deep nesting should be allowed: %v", err)
	}
	got, err := svc.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Errorf("office child ParentID = %q, want %q", got.ParentID, parent.ID)
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
