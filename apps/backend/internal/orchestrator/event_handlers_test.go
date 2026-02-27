package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// --- Mocks ---

// mockStepGetter implements WorkflowStepGetter for testing.
type mockStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep // stepID -> step
}

func newMockStepGetter() *mockStepGetter {
	return &mockStepGetter{steps: make(map[string]*wfmodels.WorkflowStep)}
}

func (m *mockStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if s, ok := m.steps[stepID]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockStepGetter) GetNextStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	var best *wfmodels.WorkflowStep
	for _, s := range m.steps {
		if s.WorkflowID == workflowID && s.Position > currentPosition {
			if best == nil || s.Position < best.Position {
				best = s
			}
		}
	}
	return best, nil
}

func (m *mockStepGetter) GetPreviousStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	var best *wfmodels.WorkflowStep
	for _, s := range m.steps {
		if s.WorkflowID == workflowID && s.Position < currentPosition {
			if best == nil || s.Position > best.Position {
				best = s
			}
		}
	}
	return best, nil
}

// mockTaskRepo implements scheduler.TaskRepository for testing.
type mockTaskRepo struct {
	tasks         map[string]*v1.Task
	updatedStates map[string]v1.TaskState
}

func newMockTaskRepo() *mockTaskRepo {
	return &mockTaskRepo{
		tasks:         make(map[string]*v1.Task),
		updatedStates: make(map[string]v1.TaskState),
	}
}

func (m *mockTaskRepo) GetTask(_ context.Context, taskID string) (*v1.Task, error) {
	if t, ok := m.tasks[taskID]; ok {
		return t, nil
	}
	return nil, nil
}

func (m *mockTaskRepo) UpdateTaskState(_ context.Context, taskID string, state v1.TaskState) error {
	m.updatedStates[taskID] = state
	return nil
}

// mockAgentManager is a minimal mock of executor.AgentManagerClient for testing.
type mockAgentManager struct {
	isPassthrough       bool
	isAgentRunning      bool
	restartProcessCalls []string // tracks execution IDs passed to RestartAgentProcess
	restartProcessErr   error
	promptErr           error
	promptResult        *executor.PromptResult
}

func (m *mockAgentManager) LaunchAgent(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	return nil, nil
}
func (m *mockAgentManager) StartAgentProcess(_ context.Context, _ string) error { return nil }
func (m *mockAgentManager) StopAgent(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockAgentManager) StopAgentWithReason(ctx context.Context, agentExecutionID, reason string, force bool) error {
	return m.StopAgent(ctx, agentExecutionID, force)
}
func (m *mockAgentManager) PromptAgent(_ context.Context, _ string, _ string, _ []v1.MessageAttachment) (*executor.PromptResult, error) {
	if m.promptErr != nil {
		return nil, m.promptErr
	}
	if m.promptResult != nil {
		return m.promptResult, nil
	}
	return &executor.PromptResult{}, nil
}
func (m *mockAgentManager) CancelAgent(_ context.Context, _ string) error { return nil }
func (m *mockAgentManager) RespondToPermissionBySessionID(_ context.Context, _, _, _ string, _ bool) error {
	return nil
}
func (m *mockAgentManager) IsAgentRunningForSession(_ context.Context, _ string) bool {
	return m.isAgentRunning
}
func (m *mockAgentManager) ResolveAgentProfile(_ context.Context, _ string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{
		SupportsMCP: true,
	}, nil
}
func (m *mockAgentManager) RestartAgentProcess(_ context.Context, agentExecutionID string) error {
	m.restartProcessCalls = append(m.restartProcessCalls, agentExecutionID)
	return m.restartProcessErr
}
func (m *mockAgentManager) SetExecutionDescription(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockAgentManager) IsPassthroughSession(_ context.Context, _ string) bool {
	return m.isPassthrough
}
func (m *mockAgentManager) GetRemoteRuntimeStatusBySession(_ context.Context, _ string) (*executor.RemoteRuntimeStatus, error) {
	return nil, nil
}
func (m *mockAgentManager) PollRemoteStatusForRecords(_ context.Context, _ []executor.RemoteStatusPollRequest) {
}
func (m *mockAgentManager) CleanupStaleExecutionBySessionID(_ context.Context, _ string) error {
	return nil
}
func (m *mockAgentManager) EnsureWorkspaceExecutionForSession(_ context.Context, _, _ string) error {
	return nil
}

// --- Helpers ---

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "console",
	})
	return log
}

func strPtr(s string) *string { return &s }

// setupTestRepo creates a real in-memory SQLite repository for testing.
func setupTestRepo(t *testing.T) *sqliterepo.Repository {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	return repo
}

// seedSession creates a task, workspace, workflow and session in the repo for testing.
func seedSession(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, workflowStepID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	// Create workspace
	ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	// Create workflow
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWorkflow(ctx, wf); err != nil {
		// Might already exist
		_ = err
	}

	// Create task
	task := &models.Task{
		ID:             taskID,
		WorkflowID:     "wf1",
		WorkflowStepID: workflowStepID,
		Title:          "Test Task",
		Description:    "Test",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Create session
	session := &models.TaskSession{
		ID:             sessionID,
		TaskID:         taskID,
		State:          models.TaskSessionStateRunning,
		WorkflowStepID: strPtr(workflowStepID),
		StartedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create task session: %v", err)
	}
}

// createTestService creates a Service with minimal dependencies for event handler testing.
func createTestService(repo *sqliterepo.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo) *Service {
	return createTestServiceWithAgent(repo, stepGetter, taskRepo, &mockAgentManager{})
}

func createTestServiceWithAgent(repo *sqliterepo.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo, agentMgr executor.AgentManagerClient) *Service {
	log := testLogger()
	return &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewService(log),
	}
}

// --- Tests ---

func TestWasResumeAttempt(t *testing.T) {
	ctx := context.Background()

	t.Run("returns true when resume token exists", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateTask(ctx, task)
		session := &models.TaskSession{ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
		_ = repo.CreateTaskSession(ctx, session)
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "acp-session-123",
			CreatedAt: now, UpdatedAt: now,
		})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if !svc.wasResumeAttempt(ctx, "s1") {
			t.Error("expected wasResumeAttempt to return true when resume token exists")
		}
	})

	t.Run("returns false when no resume token", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "t1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateTask(ctx, task)
		session := &models.TaskSession{ID: "s1", TaskID: "t1", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
		_ = repo.CreateTaskSession(ctx, session)
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "",
			CreatedAt: now, UpdatedAt: now,
		})

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if svc.wasResumeAttempt(ctx, "s1") {
			t.Error("expected wasResumeAttempt to return false when no resume token")
		}
	})

	t.Run("returns false when no executor running record", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if svc.wasResumeAttempt(ctx, "nonexistent-session") {
			t.Error("expected wasResumeAttempt to return false when no executor running record")
		}
	})
}

func TestHandleCompleteStreamEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("does not force waiting when session is still running", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		payload := &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			Data: &lifecycle.AgentStreamEventData{
				Type: agentEventComplete,
			},
		}

		svc.handleCompleteStreamEvent(ctx, payload)

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to load session: %v", err)
		}
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("expected session to stay %q, got %q", models.TaskSessionStateRunning, session.State)
		}
		if _, ok := taskRepo.updatedStates["t1"]; ok {
			t.Fatalf("expected task state to remain unchanged, got update %q", taskRepo.updatedStates["t1"])
		}
	})
}

func TestHandleAgentReadyGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("ignores ready when session is not running", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateWaitingForInput
		_ = repo.UpdateTaskSession(ctx, session)

		if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued", "", "test", false, nil); err != nil {
			t.Fatalf("failed to queue message: %v", err)
		}

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.WorkflowStepID == nil || *updated.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %v", updated.WorkflowStepID)
		}
		status := svc.messageQueue.GetStatus(ctx, "s1")
		if !status.IsQueued {
			t.Fatalf("expected queued message to remain queued")
		}
	})

	t.Run("moves STARTING session to waiting for input", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateStarting
		_ = repo.UpdateTaskSession(ctx, session)

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
		}
		if state, ok := taskRepo.updatedStates["t1"]; !ok || state != v1.TaskStateReview {
			t.Fatalf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
		}
	})

	t.Run("ignores ready while reset is in progress", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		svc.setSessionResetInProgress("s1", true)
		defer svc.setSessionResetInProgress("s1", false)

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.WorkflowStepID == nil || *updated.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %v", updated.WorkflowStepID)
		}
	})

	t.Run("ignores stale ready from old execution", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.AgentExecutionID = "exec-active"
		_ = repo.UpdateTaskSession(ctx, session)

		svc.handleAgentReady(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-stale",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.WorkflowStepID == nil || *updated.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %v", updated.WorkflowStepID)
		}
	})
}

func TestExecuteQueuedMessage_RequeuesTransientPromptFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptErr:      errors.New("agent stream disconnected: read tcp [::1]:56463->[::1]:10002: use of closed network connection"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "hello",
		QueuedBy:  "test",
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if !status.IsQueued || status.Message == nil {
		t.Fatalf("expected queued message to be requeued after transient failure")
	}
	if status.Message.Content != "hello" {
		t.Fatalf("expected queued content to be preserved, got %q", status.Message.Content)
	}
}
