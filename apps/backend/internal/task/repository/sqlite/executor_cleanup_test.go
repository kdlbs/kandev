package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestListExecutorsRunningByTaskIDIncludesTerminalSessions(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedExecutorRunningCleanupTask(t, repo, "task-1")
	seedExecutorRunningCleanupTask(t, repo, "task-2")
	seedExecutorRunningCleanupSession(t, repo, "task-1", "session-running", models.TaskSessionStateRunning, "exec-running")
	seedExecutorRunningCleanupSession(t, repo, "task-1", "session-completed", models.TaskSessionStateCompleted, "exec-completed")
	seedExecutorRunningCleanupSession(t, repo, "task-2", "session-other", models.TaskSessionStateRunning, "exec-other")

	rows, err := repo.ListExecutorsRunningByTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("ListExecutorsRunningByTaskID: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for task-1, got %d", len(rows))
	}

	got := map[string]string{}
	for _, row := range rows {
		got[row.SessionID] = row.AgentExecutionID
	}
	if got["session-running"] != "exec-running" {
		t.Fatalf("running session row missing or wrong: %#v", got)
	}
	if got["session-completed"] != "exec-completed" {
		t.Fatalf("completed session row missing or wrong: %#v", got)
	}
}

func seedExecutorRunningCleanupTask(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + taskID, Name: "Workspace " + taskID}); err != nil {
		t.Fatalf("CreateWorkspace(%s): %v", taskID, err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + taskID, WorkspaceID: "ws-" + taskID, Name: "Workflow " + taskID}); err != nil {
		t.Fatalf("CreateWorkflow(%s): %v", taskID, err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "ws-" + taskID, WorkflowID: "wf-" + taskID, WorkflowStepID: "step-" + taskID, Title: taskID, Priority: "medium"}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
}

func seedExecutorRunningCleanupSession(t *testing.T, repo *Repository, taskID, sessionID string, state models.TaskSessionState, executionID string) {
	t.Helper()
	ctx := context.Background()

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: state}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               sessionID,
		SessionID:        sessionID,
		TaskID:           taskID,
		ExecutorID:       "executor-1",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusStarting,
		AgentExecutionID: executionID,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(%s): %v", sessionID, err)
	}
}
