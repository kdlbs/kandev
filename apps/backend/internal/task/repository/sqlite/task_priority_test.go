package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestUpdateTaskPriorityOnlyChangesPriority(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-priority")

	task := &models.Task{
		ID:          "task-priority",
		WorkspaceID: "workspace-priority",
		Title:       "Keep this title",
		Description: "Keep this description",
		Priority:    "medium",
		Metadata:    map[string]interface{}{"source": "test"},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := repo.UpdateTaskPriority(ctx, task.ID, "critical"); err != nil {
		t.Fatalf("UpdateTaskPriority: %v", err)
	}
	got, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Priority != "critical" {
		t.Errorf("priority = %q, want critical", got.Priority)
	}
	if got.Title != task.Title {
		t.Errorf("title = %q, want %q", got.Title, task.Title)
	}
	if got.Description != task.Description {
		t.Errorf("description = %q, want %q", got.Description, task.Description)
	}
	if got.Metadata["source"] != "test" {
		t.Errorf("metadata = %#v, want source=test", got.Metadata)
	}

	err = repo.UpdateTaskPriority(ctx, "missing-priority-task", "low")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing task error = %v, want ErrTaskNotFound", err)
	}
}
