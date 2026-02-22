package orchestrator

import (
	"context"
	"testing"
	"time"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedTaskAndSession inserts a workspace, workflow, task, and session with the given state.
func seedTaskAndSession(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string, sessionState models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)

	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)

	task := &models.Task{
		ID:          taskID,
		WorkflowID:  "wf1",
		Title:       "Test Task",
		Description: "desc",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	session := &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     sessionState,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
}

// --- PromptTask ---

func TestPromptTask_EmptySessionID(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	_, err := svc.PromptTask(context.Background(), "task1", "", "hello", "", false, nil)
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

func TestPromptTask_SessionAlreadyRunning(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	_, err := svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil)
	if err == nil {
		t.Fatal("expected error when session is already RUNNING")
	}
}

// --- StartCreatedSession ---

func TestStartCreatedSession_WrongTask(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Session belongs to "task-other", not "task1"
	seedTaskAndSession(t, repo, "task-other", "session1", models.TaskSessionStateCreated)

	_, err := svc.StartCreatedSession(context.Background(), "task1", "session1", "profile1", "prompt")
	if err == nil {
		t.Fatal("expected error when session does not belong to task")
	}
}

func TestStartCreatedSession_NotInCreatedState(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	_, err := svc.StartCreatedSession(context.Background(), "task1", "session1", "profile1", "prompt")
	if err == nil {
		t.Fatal("expected error when session is not in CREATED state")
	}
}

// --- ResumeTaskSession ---

func TestResumeTaskSession_WrongTask(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task-other", "session1", models.TaskSessionStateWaitingForInput)

	_, err := svc.ResumeTaskSession(context.Background(), "task1", "session1")
	if err == nil {
		t.Fatal("expected error when session does not belong to task")
	}
}

func TestResumeTaskSession_NotResumable(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Session exists and belongs to task, but there is no ExecutorRunning record
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	_, err := svc.ResumeTaskSession(context.Background(), "task1", "session1")
	if err == nil {
		t.Fatal("expected error when no executor running record exists")
	}
}

// --- CompleteTask ---

func TestCompleteTask_UpdatesTaskState(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{}
	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = exec

	err := svc.CompleteTask(context.Background(), "task1")
	if err != nil {
		t.Fatalf("CompleteTask returned unexpected error: %v", err)
	}

	if state, ok := taskRepo.updatedStates["task1"]; !ok || state != v1.TaskStateCompleted {
		t.Errorf("expected task state COMPLETED, got %v (ok=%v)", state, ok)
	}
}
