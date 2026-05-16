package engine_adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// fakeParentRepo returns a static parent task or error.
type fakeParentRepo struct {
	task *taskmodels.Task
	err  error
}

func (f *fakeParentRepo) GetTask(_ context.Context, _ string) (*taskmodels.Task, error) {
	return f.task, f.err
}

// fakeChildCreator records calls.
type fakeChildCreator struct {
	calls []struct {
		Parent *taskmodels.Task
		Spec   ChildTaskCreateSpec
	}
	id  string
	err error
}

func (f *fakeChildCreator) CreateChildTask(
	_ context.Context, parent *taskmodels.Task, spec ChildTaskCreateSpec,
) (string, error) {
	f.calls = append(f.calls, struct {
		Parent *taskmodels.Task
		Spec   ChildTaskCreateSpec
	}{Parent: parent, Spec: spec})
	if f.err != nil {
		return "", f.err
	}
	if f.id == "" {
		return "child-id", nil
	}
	return f.id, nil
}

func TestTaskCreatorAdapter_HappyPath(t *testing.T) {
	parent := &taskmodels.Task{
		ID:                     "parent-1",
		WorkspaceID:            "ws-1",
		WorkflowID:             "wf-default",
		AssigneeAgentProfileID: "agent-default",
	}
	creator := &fakeChildCreator{id: "child-42"}
	a := NewTaskCreatorAdapter(&fakeParentRepo{task: parent}, creator)
	id, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{
		Title:          "Implement /healthz",
		Description:    "Add a /healthz endpoint",
		WorkflowID:     "wf-kanban",
		StepID:         "step-1",
		AgentProfileID: "profile-claude",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "child-42" {
		t.Errorf("id = %q, want child-42", id)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 CreateChildTask call, got %d", len(creator.calls))
	}
	got := creator.calls[0]
	if got.Parent != parent {
		t.Errorf("parent passed by reference mismatched: got %+v", got.Parent)
	}
	if got.Spec.Title != "Implement /healthz" {
		t.Errorf("title = %q", got.Spec.Title)
	}
	if got.Spec.WorkflowID != "wf-kanban" {
		t.Errorf("workflow_id = %q (should pass through caller's override)", got.Spec.WorkflowID)
	}
}

func TestTaskCreatorAdapter_RequiresParentID(t *testing.T) {
	a := NewTaskCreatorAdapter(&fakeParentRepo{}, &fakeChildCreator{})
	_, err := a.CreateChildTask(context.Background(), "", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "parent_task_id") {
		t.Fatalf("expected parent_task_id error, got: %v", err)
	}
}

func TestTaskCreatorAdapter_RequiresTaskService(t *testing.T) {
	a := NewTaskCreatorAdapter(&fakeParentRepo{}, nil)
	_, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "task service") {
		t.Fatalf("expected task service error, got: %v", err)
	}
}

func TestTaskCreatorAdapter_BubblesParentLookupError(t *testing.T) {
	repoErr := errors.New("boom")
	a := NewTaskCreatorAdapter(&fakeParentRepo{err: repoErr}, &fakeChildCreator{})
	_, err := a.CreateChildTask(context.Background(), "parent-1", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !errors.Is(err, repoErr) {
		t.Fatalf("expected repo error to bubble, got: %v", err)
	}
}

func TestTaskCreatorAdapter_ReturnsErrorWhenParentNotFound(t *testing.T) {
	a := NewTaskCreatorAdapter(&fakeParentRepo{task: nil}, &fakeChildCreator{})
	_, err := a.CreateChildTask(context.Background(), "missing", engine.ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}
