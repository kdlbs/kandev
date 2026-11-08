package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedRetiredTaskEnvironment creates a worktree environment and marks it as
// retained history the way failed-environment replacement does.
func seedRetiredTaskEnvironment(t *testing.T, repo *Repository, suffix string) *models.TaskEnvironment {
	t.Helper()
	ctx := context.Background()
	workspaceID := "workspace-retired-" + suffix
	taskID := "task-retired-" + suffix
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	env := &models.TaskEnvironment{
		ID:            "env-retired-" + suffix,
		TaskID:        taskID,
		ExecutorType:  string(models.ExecutorTypeWorktree),
		Status:        models.TaskEnvironmentStatusFailed,
		WorkspacePath: "/tmp/tasks/retired-" + suffix,
	}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_environments SET retired_at = ? WHERE id = ?`), time.Now().UTC(), env.ID); err != nil {
		t.Fatalf("retire environment: %v", err)
	}
	return env
}

func TestUpdateTaskEnvironmentLeavesRetiredHistoryUntouched(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	env := seedRetiredTaskEnvironment(t, repo, "update")

	env.Status = models.TaskEnvironmentStatusStopped
	err := repo.UpdateTaskEnvironment(ctx, env)
	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("UpdateTaskEnvironment(retired) error = %v, want ErrTaskEnvironmentNotFound", err)
	}
	stored, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if stored.Status != models.TaskEnvironmentStatusFailed || !stored.IsRetired() {
		t.Fatalf("retired environment = status %q retired %t, want unchanged failed history", stored.Status, stored.IsRetired())
	}
}

func TestSetTaskEnvironmentTaskDirNameIfEmptyRefusesRetiredHistory(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	env := seedRetiredTaskEnvironment(t, repo, "task-dir")

	claimed, err := repo.SetTaskEnvironmentTaskDirNameIfEmpty(ctx, env.ID, "task-root_retired")
	if !errors.Is(err, ErrTaskEnvironmentNotFound) || claimed {
		t.Fatalf("SetTaskEnvironmentTaskDirNameIfEmpty(retired) = %t, %v; want false, ErrTaskEnvironmentNotFound", claimed, err)
	}
	stored, err := repo.GetTaskEnvironment(ctx, env.ID)
	if err != nil {
		t.Fatalf("GetTaskEnvironment: %v", err)
	}
	if stored.TaskDirName != "" {
		t.Fatalf("retired TaskDirName = %q, want unchanged empty", stored.TaskDirName)
	}
}
