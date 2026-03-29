package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
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

	mu                      sync.Mutex
	stopAgentWithReasonArgs []stopAgentCall // tracks StopAgentWithReason calls

	// Passthrough stdin tracking
	passthroughStdinCalls []passthroughStdinCall
	passthroughStdinErr   error
	markPassthroughCalls  []string // session IDs
	markPassthroughErr    error
}

type stopAgentCall struct {
	ExecutionID string
	Reason      string
	Force       bool
}

type passthroughStdinCall struct {
	SessionID string
	Data      string
}

func (m *mockAgentManager) LaunchAgent(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	return nil, nil
}
func (m *mockAgentManager) StartAgentProcess(_ context.Context, _ string) error { return nil }
func (m *mockAgentManager) StopAgent(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockAgentManager) StopAgentWithReason(_ context.Context, agentExecutionID, reason string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopAgentWithReasonArgs = append(m.stopAgentWithReasonArgs, stopAgentCall{
		ExecutionID: agentExecutionID,
		Reason:      reason,
		Force:       force,
	})
	return nil
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
func (m *mockAgentManager) ResetAgentContext(ctx context.Context, agentExecutionID string) error {
	return m.RestartAgentProcess(ctx, agentExecutionID)
}
func (m *mockAgentManager) SetExecutionDescription(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockAgentManager) SetSessionModelBySessionID(_ context.Context, _, _ string) error {
	return fmt.Errorf("not supported")
}
func (m *mockAgentManager) SetMcpMode(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *mockAgentManager) WasSessionInitialized(_ string) bool { return false }
func (m *mockAgentManager) GetSessionAuthMethods(_ string) []streams.AuthMethodInfo {
	return nil
}
func (m *mockAgentManager) IsPassthroughSession(_ context.Context, _ string) bool {
	return m.isPassthrough
}
func (m *mockAgentManager) WritePassthroughStdin(_ context.Context, sessionID string, data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.passthroughStdinCalls = append(m.passthroughStdinCalls, passthroughStdinCall{SessionID: sessionID, Data: data})
	return m.passthroughStdinErr
}
func (m *mockAgentManager) MarkPassthroughRunning(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markPassthroughCalls = append(m.markPassthroughCalls, sessionID)
	return m.markPassthroughErr
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
func (m *mockAgentManager) GetExecutionIDForSession(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("no execution found")
}
func (m *mockAgentManager) GetGitLog(_ context.Context, _, _ string, _ int) (*client.GitLogResult, error) {
	return nil, nil
}
func (m *mockAgentManager) GetCumulativeDiff(_ context.Context, _, _ string) (*client.CumulativeDiffResult, error) {
	return nil, nil
}
func (m *mockAgentManager) GetGitStatus(_ context.Context, _ string) (*client.GitStatusResult, error) {
	return &client.GitStatusResult{
		Success:    true,
		Branch:     "main",
		HeadCommit: "mock-commit",
	}, nil
}
func (m *mockAgentManager) WaitForAgentctlReady(_ context.Context, _ string) error {
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
		ID:        sessionID,
		TaskID:    taskID,
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
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

		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %q", updatedTask.WorkflowStepID)
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

		// Register the workflow step so processOnTurnComplete can resolve it.
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}
		svc := createTestService(repo, stepGetter, taskRepo)

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

		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %q", updatedTask.WorkflowStepID)
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

		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step1" {
			t.Fatalf("expected workflow step to remain step1, got %q", updatedTask.WorkflowStepID)
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

func TestExecuteQueuedMessage_FiresOnTurnStart(t *testing.T) {
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

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{
				{Type: wfmodels.OnTurnStartMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{},
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		// PromptAgent succeeds so the message is consumed normally.
	}
	log := testLogger()
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     taskRepo,
		agentManager: agentMgr,
		messageQueue: messagequeue.NewService(log),
	}
	svc.SetWorkflowStepGetter(stepGetter)
	svc.executor = executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "auto-start prompt",
		QueuedBy:  "workflow-auto-start",
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	// Verify on_turn_start moved the task from step1 to step2.
	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step2" {
		t.Errorf("expected task workflow step to be 'step2', got %q", updatedTask.WorkflowStepID)
	}
}

func TestExecuteQueuedMessage_NoOnTurnStart_StepUnchanged(t *testing.T) {
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

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{}, // no on_turn_start
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true}
	log := testLogger()
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     taskRepo,
		agentManager: agentMgr,
		messageQueue: messagequeue.NewService(log),
	}
	svc.SetWorkflowStepGetter(stepGetter)
	svc.executor = executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "auto-start prompt",
		QueuedBy:  "workflow-auto-start",
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	// Verify task stayed on step1 (no on_turn_start actions).
	updatedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updatedTask.WorkflowStepID != "step1" {
		t.Errorf("expected task workflow step to remain 'step1', got %q", updatedTask.WorkflowStepID)
	}
}

func createTestServiceWithScheduler(repo *sqliterepo.Repository, stepGetter *mockStepGetter, taskRepo *mockTaskRepo, agentMgr executor.AgentManagerClient) *Service {
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})
	return &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewService(log),
		executor:           exec,
		scheduler:          sched,
	}
}

func TestHandleAgentCompleted_CleansUpExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
	})

	// cleanupAgentExecution runs in a goroutine; give it time to execute
	waitForStopCall(t, agentMgr)

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	call := agentMgr.stopAgentWithReasonArgs[0]
	if call.ExecutionID != "exec-1" {
		t.Errorf("expected execution ID %q, got %q", "exec-1", call.ExecutionID)
	}
	if !call.Force {
		t.Error("expected force=true for cleanup after completion")
	}
}

func TestHandleAgentFailed_CleansUpExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "")

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	// cleanupAgentExecution runs in a goroutine; give it time to execute
	waitForStopCall(t, agentMgr)

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	call := agentMgr.stopAgentWithReasonArgs[0]
	if call.ExecutionID != "exec-1" {
		t.Errorf("expected execution ID %q, got %q", "exec-1", call.ExecutionID)
	}
}

func TestCleanupAgentExecution_SkipsEmptyExecutionID(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	// Should return immediately without calling StopAgentWithReason
	svc.cleanupAgentExecution("", "t1", "s1")

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	if len(agentMgr.stopAgentWithReasonArgs) != 0 {
		t.Error("expected no StopAgentWithReason call for empty execution ID")
	}
}

func TestHandleAgentRunning_PassthroughGuard(t *testing.T) {
	ctx := context.Background()

	t.Run("ACP session skips on_turn_start", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnStart: []wfmodels.OnTurnStartAction{
					{Type: wfmodels.OnTurnStartMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{isPassthrough: false}
		svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)

		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		// Workflow step must remain step1 because on_turn_start is skipped for ACP sessions.
		updatedTask, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updatedTask.WorkflowStepID != "step1" {
			t.Errorf("expected task workflow step to remain 'step1', got %q", updatedTask.WorkflowStepID)
		}
	})

	t.Run("passthrough session fires on_turn_start", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			Events: wfmodels.StepEvents{
				OnTurnStart: []wfmodels.OnTurnStartAction{
					{Type: wfmodels.OnTurnStartMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)

		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		// Workflow step must move to step2 because passthrough sessions fire on_turn_start.
		updatedTask, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updatedTask.WorkflowStepID != "step2" {
			t.Errorf("expected task workflow step to be 'step2', got %q", updatedTask.WorkflowStepID)
		}
	})

	t.Run("missing session_id is ignored", func(t *testing.T) {
		repo := setupTestRepo(t)
		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		// Should not panic or error with empty session ID.
		svc.handleAgentRunning(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: ""})
	})
}

func TestDeliverPassthroughPrompt(t *testing.T) {
	ctx := context.Background()

	t.Run("writes to stdin and marks running", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		err := svc.deliverPassthroughPrompt(ctx, "s1", "hello")
		if err != nil {
			t.Fatalf("deliverPassthroughPrompt returned error: %v", err)
		}

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		if len(agentMgr.passthroughStdinCalls) != 1 {
			t.Fatalf("expected 1 stdin call, got %d", len(agentMgr.passthroughStdinCalls))
		}
		call := agentMgr.passthroughStdinCalls[0]
		if call.SessionID != "s1" {
			t.Errorf("stdin sessionID = %q, want %q", call.SessionID, "s1")
		}
		if call.Data != "hello\r" {
			t.Errorf("stdin data = %q, want %q", call.Data, "hello\r")
		}
		if len(agentMgr.markPassthroughCalls) != 1 || agentMgr.markPassthroughCalls[0] != "s1" {
			t.Errorf("markPassthroughRunning calls = %v, want [s1]", agentMgr.markPassthroughCalls)
		}
	})

	t.Run("returns error when stdin write fails", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{
			isPassthrough:       true,
			passthroughStdinErr: fmt.Errorf("stdin write failed"),
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		err := svc.deliverPassthroughPrompt(ctx, "s1", "hello")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		// Should not call markPassthroughRunning when stdin write fails
		if len(agentMgr.markPassthroughCalls) != 0 {
			t.Errorf("markPassthroughRunning should not be called when stdin fails, got %d calls", len(agentMgr.markPassthroughCalls))
		}
	})
}

func TestHandleAgentReady_PassthroughQueuedMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("delivers queued message to passthrough via stdin", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set session to RUNNING so handleAgentReady doesn't early-return
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateRunning
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)

		// Queue a message
		if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "queued prompt", "", "test", false, nil); err != nil {
			t.Fatalf("failed to queue message: %v", err)
		}

		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		// Verify the queued message was delivered to passthrough stdin
		if len(agentMgr.passthroughStdinCalls) != 1 {
			t.Fatalf("expected 1 stdin call, got %d", len(agentMgr.passthroughStdinCalls))
		}
		call := agentMgr.passthroughStdinCalls[0]
		if call.Data != "queued prompt\r" {
			t.Errorf("stdin data = %q, want %q", call.Data, "queued prompt\r")
		}

		// Queue should be empty after delivery
		status := svc.messageQueue.GetStatus(ctx, "s1")
		if status.IsQueued {
			t.Error("expected queue to be empty after delivery")
		}
	})

	t.Run("skips delivery when no queued message exists", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateRunning
		_ = repo.UpdateTaskSession(ctx, session)

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		}

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentMgr)

		// No queued message — should return early
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		if len(agentMgr.passthroughStdinCalls) != 0 {
			t.Errorf("expected no stdin calls, got %d", len(agentMgr.passthroughStdinCalls))
		}
	})
}

func TestAutoStartPassthroughPrompt(t *testing.T) {
	ctx := context.Background()

	t.Run("writes prompt to stdin and logs step name", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{isPassthrough: true}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		session, _ := repo.GetTaskSession(ctx, "s1")
		err := svc.autoStartPassthroughPrompt(ctx, "t1", session, "Analyze", "do analysis")
		if err != nil {
			t.Fatalf("autoStartPassthroughPrompt returned error: %v", err)
		}

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()

		if len(agentMgr.passthroughStdinCalls) != 1 {
			t.Fatalf("expected 1 stdin call, got %d", len(agentMgr.passthroughStdinCalls))
		}
		if agentMgr.passthroughStdinCalls[0].Data != "do analysis\r" {
			t.Errorf("stdin data = %q, want %q", agentMgr.passthroughStdinCalls[0].Data, "do analysis\r")
		}
	})

	t.Run("returns error when stdin write fails", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		agentMgr := &mockAgentManager{
			isPassthrough:       true,
			passthroughStdinErr: fmt.Errorf("stdin write failed"),
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

		session, _ := repo.GetTaskSession(ctx, "s1")
		err := svc.autoStartPassthroughPrompt(ctx, "t1", session, "Analyze", "do analysis")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestClearResumeToken(t *testing.T) {
	ctx := context.Background()

	t.Run("clears existing resume token", func(t *testing.T) {
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
		svc.clearResumeToken(ctx, "s1")

		running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get executor running: %v", err)
		}
		if running.ResumeToken != "" {
			t.Errorf("expected empty resume token, got %q", running.ResumeToken)
		}
	})

	t.Run("no-op when no executor running record", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		// Should not panic
		svc.clearResumeToken(ctx, "nonexistent-session")
	})

	t.Run("no-op when token already empty", func(t *testing.T) {
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
		// Should not panic or error
		svc.clearResumeToken(ctx, "s1")
	})
}

func TestHandleRecoverableFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("sets session to WAITING_FOR_INPUT with error message", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent crashed unexpectedly",
		})

		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
		if session.ErrorMessage != "agent crashed unexpectedly" {
			t.Errorf("expected error message %q, got %q", "agent crashed unexpectedly", session.ErrorMessage)
		}
	})

	t.Run("sets task to REVIEW state", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent crashed",
		})

		if state, ok := taskRepo.updatedStates["t1"]; !ok || state != v1.TaskStateReview {
			t.Errorf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
		}
	})

	t.Run("cleans up agent execution", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleRecoverableFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent crashed",
		})

		waitForStopCall(t, agentMgr)

		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()
		if len(agentMgr.stopAgentWithReasonArgs) == 0 {
			t.Error("expected cleanup to call StopAgentWithReason")
		}
	})
}

func TestHandleResumeFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("clears resume token and sets WAITING_FOR_INPUT", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		seedSession(t, repo, "t1", "s1", "step1")

		// Add executor running with resume token
		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "acp-session-old",
			CreatedAt: now, UpdatedAt: now,
		})

		taskRepo := newMockTaskRepo()
		svc := createTestService(repo, newMockStepGetter(), taskRepo)

		result := svc.handleResumeFailure(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "resume failed: session expired",
		})

		if !result {
			t.Error("expected handleResumeFailure to return true")
		}

		// Verify resume token was cleared
		running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get executor running: %v", err)
		}
		if running.ResumeToken != "" {
			t.Errorf("expected empty resume token, got %q", running.ResumeToken)
		}

		// Verify session state
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}

		// Verify task moved to REVIEW
		if state, ok := taskRepo.updatedStates["t1"]; !ok || state != v1.TaskStateReview {
			t.Errorf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
		}
	})
}

func TestHandleAgentFailed_RecoverableWithSession(t *testing.T) {
	ctx := context.Background()

	t.Run("routes to recoverable failure when session exists", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{}
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleAgentFailed(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "agent process exited",
		})

		// Should set session to WAITING_FOR_INPUT (recoverable), not FAILED
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected recoverable state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
	})

	t.Run("routes to resume failure when resume token exists and init not completed", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		seedSession(t, repo, "t1", "s1", "step1")

		_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "er1", SessionID: "s1", TaskID: "t1", ResumeToken: "acp-session-old",
			CreatedAt: now, UpdatedAt: now,
		})

		taskRepo := newMockTaskRepo()
		agentMgr := &mockAgentManager{} // WasSessionInitialized returns false by default
		svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

		svc.handleAgentFailed(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
			ErrorMessage:     "resume failed",
		})

		// Resume token should be cleared
		running, err := repo.GetExecutorRunningBySessionID(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get executor running: %v", err)
		}
		if running.ResumeToken != "" {
			t.Errorf("expected resume token to be cleared, got %q", running.ResumeToken)
		}

		// Session should be WAITING_FOR_INPUT
		session, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		if session.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, session.State)
		}
	})
}

func TestHandleAgentStopped_PreservesRecoveryState(t *testing.T) {
	ctx := context.Background()

	t.Run("does not clobber WAITING_FOR_INPUT state", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		// Set session to WAITING_FOR_INPUT (recovery state)
		session, _ := repo.GetTaskSession(ctx, "s1")
		session.State = models.TaskSessionStateWaitingForInput
		_ = repo.UpdateTaskSession(ctx, session)

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.handleAgentStopped(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateWaitingForInput {
			t.Errorf("expected state to remain %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
		}
	})

	t.Run("sets CANCELLED when session is not in recovery", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

		svc.handleAgentStopped(ctx, watcher.AgentEventData{
			TaskID:           "t1",
			SessionID:        "s1",
			AgentExecutionID: "exec-1",
		})

		updated, _ := repo.GetTaskSession(ctx, "s1")
		if updated.State != models.TaskSessionStateCancelled {
			t.Errorf("expected state %q, got %q", models.TaskSessionStateCancelled, updated.State)
		}
	})
}

// waitForStopCall polls until the mock agent manager has received at least one
// StopAgentWithReason call, or fails the test after a timeout.
func waitForStopCall(t *testing.T, agentMgr *mockAgentManager) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		agentMgr.mu.Lock()
		calls := len(agentMgr.stopAgentWithReasonArgs)
		agentMgr.mu.Unlock()
		if calls > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("expected StopAgentWithReason to be called, but it was not")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
