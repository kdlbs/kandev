package sqlite

import (
	"context"
	"testing"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/task/models"
)

// TestListEphemeralTasksAllWorkspacesSpansWorkspaces pins the whole reason
// this method exists over ListTasksByWorkspace: a plugin's managed
// conversations can live in multiple workspaces, and uninstall must find all
// of them in one query rather than iterating every workspace by hand.
func TestListEphemeralTasksAllWorkspacesSpansWorkspaces(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-a")
	seedWorkspace(t, repo, "ws-b")

	ephemeralA := ephemeralTaskFixture("task-ephemeral-a", "ws-a")
	ephemeralB := ephemeralTaskFixture("task-ephemeral-b", "ws-b")
	ordinaryA := ephemeralTaskFixture("task-ordinary-a", "ws-a")
	ordinaryA.IsEphemeral = false

	for _, task := range []*models.Task{ephemeralA, ephemeralB, ordinaryA} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	got, err := repo.ListEphemeralTasksAllWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListEphemeralTasksAllWorkspaces: %v", err)
	}

	ids := make(map[string]bool)
	for _, task := range got {
		ids[task.ID] = true
	}
	if !ids["task-ephemeral-a"] {
		t.Error("expected task-ephemeral-a (ws-a) in results")
	}
	if !ids["task-ephemeral-b"] {
		t.Error("expected task-ephemeral-b (ws-b), a different workspace, in results")
	}
	if ids["task-ordinary-a"] {
		t.Error("expected the non-ephemeral task to be excluded")
	}
}

func ephemeralTaskFixture(id, workspaceID string) *models.Task {
	now := time.Now().UTC()
	return &models.Task{
		ID:          id,
		WorkspaceID: workspaceID,
		Title:       "Managed Conversation - " + id,
		State:       v1.TaskStateCreated,
		Priority:    "medium",
		IsEphemeral: true,
		Origin:      models.TaskOriginManual,
		Metadata: map[string]interface{}{
			"kandev.plugin_id": "plugin-coordinator",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
