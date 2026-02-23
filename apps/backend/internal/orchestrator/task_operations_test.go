package orchestrator

import (
	"context"
	"errors"
	"fmt"
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

func TestPromptTask_TransientErrorDoesNotMoveTaskToReview(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptErr:      errors.New("agent stream disconnected: read tcp [::1]:56463->[::1]:10002: use of closed network connection"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	_, err = svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil)
	if err == nil {
		t.Fatal("expected transient prompt error")
	}

	if got, ok := taskRepo.updatedStates["task1"]; ok && got == v1.TaskStateReview {
		t.Fatalf("expected task state to avoid REVIEW on transient prompt error, got %q", got)
	}

	updated, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
	}
}

func TestPromptTask_ResetInProgressReturnsSentinelError(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	svc.setSessionResetInProgress("session1", true)
	defer svc.setSessionResetInProgress("session1", false)

	_, err := svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil)
	if !errors.Is(err, ErrSessionResetInProgress) {
		t.Fatalf("expected ErrSessionResetInProgress, got %v", err)
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

// --- Error Classification Functions ---

func TestErrorClassificationFunctions(t *testing.T) {
	t.Run("isAgentPromptInProgressError", func(t *testing.T) {
		tests := []struct {
			name string
			err  error
			want bool
		}{
			{"nil error", nil, false},
			{"unrelated error", errors.New("something else"), false},
			{"exact match", ErrAgentPromptInProgress, true},
			{"wrapped error", fmt.Errorf("outer: %w", ErrAgentPromptInProgress), true},
			{"string contains match", errors.New("prefix: agent is currently processing a prompt, try later"), true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := isAgentPromptInProgressError(tc.err); got != tc.want {
					t.Errorf("isAgentPromptInProgressError(%v) = %v, want %v", tc.err, got, tc.want)
				}
			})
		}
	})

	t.Run("isSessionResetInProgressError", func(t *testing.T) {
		tests := []struct {
			name string
			err  error
			want bool
		}{
			{"nil error", nil, false},
			{"unrelated error", errors.New("something else"), false},
			{"exact match", ErrSessionResetInProgress, true},
			{"wrapped error", fmt.Errorf("outer: %w", ErrSessionResetInProgress), true},
			{"string contains match", errors.New("prefix: session reset in progress, please wait"), true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := isSessionResetInProgressError(tc.err); got != tc.want {
					t.Errorf("isSessionResetInProgressError(%v) = %v, want %v", tc.err, got, tc.want)
				}
			})
		}
	})

	t.Run("isTransientPromptError", func(t *testing.T) {
		tests := []struct {
			name string
			err  error
			want bool
		}{
			{"nil error", nil, false},
			{"unrelated error", errors.New("something else"), false},
			{"agent stream disconnected", errors.New("agent stream disconnected: read tcp"), true},
			{"use of closed network connection", errors.New("write: use of closed network connection"), true},
			{"case insensitive match", errors.New("Agent Stream Disconnected: EOF"), true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := isTransientPromptError(tc.err); got != tc.want {
					t.Errorf("isTransientPromptError(%v) = %v, want %v", tc.err, got, tc.want)
				}
			})
		}
	})
}

// --- ReconcileSessionsOnStartup ---

func TestReconcileSessionsOnStartup(t *testing.T) {
	t.Run("terminal_session_cleaned_up", func(t *testing.T) {
		repo := setupTestRepo(t)
		ctx := context.Background()
		now := time.Now().UTC()

		seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCompleted)

		err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:        "er1",
			SessionID: "session1",
			TaskID:    "task1",
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("failed to upsert executor running: %v", err)
		}

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
		svc.reconcileSessionsOnStartup(ctx)

		_, err = repo.GetExecutorRunningBySessionID(ctx, "session1")
		if err == nil {
			t.Fatal("expected ExecutorRunning record to be deleted for terminal session")
		}
	})

	t.Run("active_session_set_to_waiting", func(t *testing.T) {
		repo := setupTestRepo(t)
		ctx := context.Background()
		now := time.Now().UTC()

		seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

		err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:        "er1",
			SessionID: "session1",
			TaskID:    "task1",
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("failed to upsert executor running: %v", err)
		}

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
		svc.reconcileSessionsOnStartup(ctx)

		session, err := repo.GetTaskSession(ctx, "session1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}

		// ExecutorRunning should be preserved for lazy resume
		_, err = repo.GetExecutorRunningBySessionID(ctx, "session1")
		if err != nil {
			t.Fatalf("expected ExecutorRunning record to be preserved, got error: %v", err)
		}
	})

	t.Run("failed_session_with_resume_token_preserved", func(t *testing.T) {
		repo := setupTestRepo(t)
		ctx := context.Background()
		now := time.Now().UTC()

		seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)

		err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:          "er1",
			SessionID:   "session1",
			TaskID:      "task1",
			ResumeToken: "acp-session-abc",
			Resumable:   true,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			t.Fatalf("failed to upsert executor running: %v", err)
		}

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["task1"] = &v1.Task{
			ID:    "task1",
			State: v1.TaskStateReview,
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
		svc.reconcileSessionsOnStartup(ctx)

		// ExecutorRunning should be preserved because it has a resume token and is resumable
		er, err := repo.GetExecutorRunningBySessionID(ctx, "session1")
		if err != nil {
			t.Fatalf("expected ExecutorRunning to be preserved for resumable failed session, got error: %v", err)
		}
		if er.ResumeToken != "acp-session-abc" {
			t.Fatalf("expected resume token to be preserved, got %q", er.ResumeToken)
		}
	})

	t.Run("task_in_progress_moved_to_review", func(t *testing.T) {
		repo := setupTestRepo(t)
		ctx := context.Background()
		now := time.Now().UTC()

		seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

		err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:        "er1",
			SessionID: "session1",
			TaskID:    "task1",
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("failed to upsert executor running: %v", err)
		}

		taskRepo := newMockTaskRepo()
		taskRepo.tasks["task1"] = &v1.Task{
			ID:    "task1",
			State: v1.TaskStateInProgress,
		}

		svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
		svc.reconcileSessionsOnStartup(ctx)

		state, ok := taskRepo.updatedStates["task1"]
		if !ok {
			t.Fatal("expected task state to be updated")
		}
		if state != v1.TaskStateReview {
			t.Fatalf("expected task state %q, got %q", v1.TaskStateReview, state)
		}
	})
}
