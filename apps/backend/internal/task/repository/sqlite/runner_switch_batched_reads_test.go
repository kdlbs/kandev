package sqlite

import (
	"context"
	"fmt"
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

// TestGetExecutorRunningExistenceByTaskIDsChunksAcrossHostParamLimit proves
// batchedTaskIDExistence's chunking merges results correctly when the
// caller-supplied task ID count exceeds sqliteMaxHostParams: a boot or board
// load for a large workflow must not error the whole read, and a hit must
// still be found regardless of which chunk it falls into.
func TestGetExecutorRunningExistenceByTaskIDsChunksAcrossHostParamLimit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskA, _ := seedRunnerBatchTasks(t, repo, "ws-exec-running-chunked")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-running-chunked-a", SessionID: "sess-exec-running-chunked-a", TaskID: taskA, ExecutorID: models.ExecutorIDLocal,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	// Pad well past sqliteMaxHostParams (500) so the read spans at least two
	// chunks, with the real hit placed last so it lands in the final chunk.
	taskIDs := make([]string, 0, sqliteMaxHostParams*2+1)
	for i := 0; i < sqliteMaxHostParams*2; i++ {
		taskIDs = append(taskIDs, fmt.Sprintf("padding-task-%d", i))
	}
	taskIDs = append(taskIDs, taskA)

	got, err := repo.GetExecutorRunningExistenceByTaskIDs(ctx, taskIDs)
	if err != nil {
		t.Fatalf("GetExecutorRunningExistenceByTaskIDs with %d IDs: %v", len(taskIDs), err)
	}
	if !got[taskA] {
		t.Errorf("task with a running executor reported false across chunked read")
	}
	if len(got) != 1 {
		t.Errorf("expected exactly one hit across chunked read, got %d: %#v", len(got), got)
	}
}
