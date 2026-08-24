package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository"
)

func TestGetTaskRelations_UsesWorkspaceAsTheVisibilityBoundary(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("foreign-parent", "", "workspace-b")
	tasks.addTask("target", "foreign-parent", "workspace-a")
	tasks.addTask("same-child", "target", "workspace-a")
	tasks.addTask("foreign-child", "target", "workspace-b")

	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())
	relations, err := svc.GetTaskRelations(context.Background(), "workspace-a", "target")
	if err != nil {
		t.Fatalf("GetTaskRelations: %v", err)
	}
	if relations.Parent != nil {
		t.Fatalf("foreign parent leaked: %+v", relations.Parent)
	}
	if len(relations.Children) != 1 || relations.Children[0].ID != "same-child" {
		t.Fatalf("children = %+v, want only same-child", relations.Children)
	}
	if relations.Children[0].Title != "" {
		t.Fatalf("unexpected sensitive projection: %+v", relations.Children[0])
	}
}

func TestGetTaskRelations_HidesForeignAndUnknownTargets(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("foreign", "", "workspace-b")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	for _, taskID := range []string{"foreign", "missing"} {
		_, err := svc.GetTaskRelations(context.Background(), "workspace-a", taskID)
		if !errors.Is(err, repository.ErrTaskNotFound) {
			t.Fatalf("GetTaskRelations(%q) error = %v, want ErrTaskNotFound", taskID, err)
		}
	}
}
