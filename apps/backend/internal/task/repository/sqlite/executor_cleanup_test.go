package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestListExecutorsRunningScansMetadataAndRuntimeFields(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	lastSeen := time.Now().UTC().Truncate(time.Second)

	seedExecutorRunningCleanupTask(t, repo, "task-1")
	seedExecutorRunningCleanupSession(t, repo, "task-1", "session-running", models.TaskSessionStateRunning, "exec-running")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-completed",
		TaskID: "task-1",
		State:  models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("CreateTaskSession(session-completed): %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session-completed",
		SessionID:        "session-completed",
		TaskID:           "task-1",
		ExecutorID:       "executor-1",
		Runtime:          agentruntime.RuntimeDocker,
		Status:           models.ExecutorRunningStatusPrepared,
		Resumable:        true,
		ResumeToken:      "resume-token",
		LastMessageUUID:  "message-1",
		AgentExecutionID: "exec-completed",
		ContainerID:      "container-1",
		AgentctlURL:      "http://127.0.0.1:1234",
		AgentctlPort:     1234,
		PID:              4321,
		WorktreeID:       "worktree-1",
		WorktreePath:     "/tmp/worktree-1",
		WorktreeBranch:   "feature/test",
		LastSeenAt:       &lastSeen,
		ErrorMessage:     "last error",
		Metadata: map[string]interface{}{
			"kind": "terminal",
		},
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning(session-completed): %v", err)
	}

	rows, err := repo.ListExecutorsRunning(ctx)
	if err != nil {
		t.Fatalf("ListExecutorsRunning: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 running rows, got %d", len(rows))
	}

	got := map[string]*models.ExecutorRunning{}
	for _, row := range rows {
		got[row.SessionID] = row
	}
	row := got["session-completed"]
	if row == nil {
		t.Fatalf("session-completed row missing: %#v", got)
	}
	if row.Runtime != agentruntime.RuntimeDocker || row.Status != models.ExecutorRunningStatusPrepared || !row.Resumable {
		t.Fatalf("runtime fields not scanned: %#v", row)
	}
	if row.ResumeToken != "resume-token" || row.LastMessageUUID != "message-1" || row.AgentExecutionID != "exec-completed" {
		t.Fatalf("execution fields not scanned: %#v", row)
	}
	if row.ContainerID != "container-1" || row.AgentctlURL != "http://127.0.0.1:1234" || row.AgentctlPort != 1234 || row.PID != 4321 {
		t.Fatalf("process fields not scanned: %#v", row)
	}
	if row.WorktreeID != "worktree-1" || row.WorktreePath != "/tmp/worktree-1" || row.WorktreeBranch != "feature/test" {
		t.Fatalf("worktree fields not scanned: %#v", row)
	}
	if row.LastSeenAt == nil || !row.LastSeenAt.Equal(lastSeen) {
		t.Fatalf("last_seen_at not scanned: got %v want %v", row.LastSeenAt, lastSeen)
	}
	if row.ErrorMessage != "last error" || row.Metadata["kind"] != "terminal" {
		t.Fatalf("metadata fields not scanned: %#v", row)
	}
}

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
