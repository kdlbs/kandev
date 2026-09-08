package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// seedRunnerBatchTasks creates a workspace and two tasks in it, returning
// their IDs.
func seedRunnerBatchTasks(t *testing.T, repo *Repository, workspaceID string) (taskA, taskB string) {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, workspaceID)
	taskA, taskB = workspaceID+"-task-a", workspaceID+"-task-b"
	for _, id := range []string{taskA, taskB} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: workspaceID, Title: id}); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}
	return taskA, taskB
}

func TestGetTaskEnvironmentExistenceByTaskIDs(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskA, taskB := seedRunnerBatchTasks(t, repo, "ws-env-existence")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-existence-a", TaskID: taskA, ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	got, err := repo.GetTaskEnvironmentExistenceByTaskIDs(ctx, []string{taskA, taskB, "task-not-in-batch"})
	if err != nil {
		t.Fatalf("GetTaskEnvironmentExistenceByTaskIDs: %v", err)
	}
	if !got[taskA] {
		t.Errorf("task with environment reported false")
	}
	if got[taskB] {
		t.Errorf("task without environment reported true")
	}
	if got["task-not-in-batch"] {
		t.Errorf("unknown task ID present in result")
	}
}

func TestGetTaskEnvironmentExistenceByTaskIDsEmptyInput(t *testing.T) {
	repo := newRepoForEntityTests(t)
	got, err := repo.GetTaskEnvironmentExistenceByTaskIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetTaskEnvironmentExistenceByTaskIDs(nil): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for empty input, got %#v", got)
	}
}

func TestGetExecutorRunningExistenceByTaskIDs(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskA, taskB := seedRunnerBatchTasks(t, repo, "ws-exec-running-existence")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-running-a", SessionID: "sess-exec-running-a", TaskID: taskA, ExecutorID: models.ExecutorIDLocal,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	got, err := repo.GetExecutorRunningExistenceByTaskIDs(ctx, []string{taskA, taskB})
	if err != nil {
		t.Fatalf("GetExecutorRunningExistenceByTaskIDs: %v", err)
	}
	if !got[taskA] {
		t.Errorf("task with a running executor reported false")
	}
	if got[taskB] {
		t.Errorf("task without a running executor reported true")
	}
}

func TestGetExecutorRunningExistenceByTaskIDsEmptyInput(t *testing.T) {
	repo := newRepoForEntityTests(t)
	got, err := repo.GetExecutorRunningExistenceByTaskIDs(context.Background(), []string{})
	if err != nil {
		t.Fatalf("GetExecutorRunningExistenceByTaskIDs([]): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for empty input, got %#v", got)
	}
}
