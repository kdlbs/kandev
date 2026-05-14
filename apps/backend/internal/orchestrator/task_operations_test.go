package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/dto"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
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
	_, err := svc.PromptTask(context.Background(), "task1", "", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

func TestPromptTask_SessionAlreadyRunning(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	_, err := svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false)
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
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	_, err = svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false)
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

// TestPromptTask_CancelEscalatedDoesNotMoveTaskToReview ensures that when the
// lifecycle manager force-unblocks a hung agent (returning ErrCancelEscalated
// wrapped in the agent-error format), PromptTask recognises it as a cancel and
// leaves the task state untouched — the user cancelled, this is not a failure
// the reviewer needs to look at. Service.CancelAgent reconciles session state
// separately; PromptTask must not race ahead with UpdateTaskState(REVIEW).
func TestPromptTask_CancelEscalatedDoesNotMoveTaskToReview(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptErr:      fmt.Errorf("agent error: cancel escalated: agent did not complete turn within timeout: %w", lifecycle.ErrCancelEscalated),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	_, err = svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected cancel-escalated error to bubble up from PromptTask")
	}
	if !errors.Is(err, lifecycle.ErrCancelEscalated) {
		t.Fatalf("expected ErrCancelEscalated, got: %v", err)
	}

	if got, ok := taskRepo.updatedStates["task1"]; ok && got == v1.TaskStateReview {
		t.Fatalf("expected task state to avoid REVIEW on cancel escalation, got %q", got)
	}
}

// TestPromptTask_ExecutionNotFoundRevertsStateAndBroadcasts ensures that when
// Prompt returns executor.ErrExecutionNotFound, PromptTask reverts the session
// state via the broadcasting wrapper (not a direct repo write), so the WS
// subscribers receive session.state_changed and the UI can unstick the
// "Agent is running" composer/pause button.
// Regression test for the stuck-UI bug after a prompt failure.
func TestPromptTask_ExecutionNotFoundRevertsStateAndBroadcasts(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptErr:      fmt.Errorf("wrapped: %w", lifecycle.ErrExecutionNotFound),
	}
	eb := &recordingEventBus{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.eventBus = eb

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	_, err = svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected error from prompt, got nil")
	}
	if !errors.Is(err, executor.ErrExecutionNotFound) {
		t.Fatalf("expected ErrExecutionNotFound bubbled up, got: %v", err)
	}

	updated, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session state WAITING_FOR_INPUT after revert, got %q", updated.State)
	}

	var sawRevert bool
	for _, evt := range eb.events {
		if evt.subject != events.TaskSessionStateChanged {
			continue
		}
		payload, ok := evt.event.Data.(map[string]interface{})
		if !ok {
			continue
		}
		oldState, _ := payload["old_state"].(string)
		newState, _ := payload["new_state"].(string)
		sessID, _ := payload["session_id"].(string)
		if sessID == "session1" && oldState == string(models.TaskSessionStateRunning) && newState == string(models.TaskSessionStateWaitingForInput) {
			sawRevert = true
			break
		}
	}
	if !sawRevert {
		t.Fatalf("expected TaskSessionStateChanged RUNNING→WAITING_FOR_INPUT broadcast after prompt failure, got events: %+v", eb.events)
	}
}

func TestPromptTask_PlanModeInjectsPrefix(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	_, err = svc.PromptTask(context.Background(), "task1", "session1", "update the plan", "", true, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(agentMgr.capturedPrompts))
	}
	if !strings.Contains(agentMgr.capturedPrompts[0], "PLAN MODE ACTIVE") {
		t.Fatalf("expected plan mode prefix in prompt, got: %s", agentMgr.capturedPrompts[0])
	}
}

func TestPromptTask_NoPlanModeDoesNotInjectPrefix(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	_, err = svc.PromptTask(context.Background(), "task1", "session1", "implement the feature", "", false, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(agentMgr.capturedPrompts))
	}
	if strings.Contains(agentMgr.capturedPrompts[0], "PLAN MODE ACTIVE") {
		t.Fatalf("expected no plan mode prefix in prompt, got: %s", agentMgr.capturedPrompts[0])
	}
}

func TestPromptTask_ResetInProgressReturnsSentinelError(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	svc.setSessionResetInProgress("session1", true)
	defer svc.setSessionResetInProgress("session1", false)

	_, err := svc.PromptTask(context.Background(), "task1", "session1", "hello", "", false, nil, false)
	if !errors.Is(err, ErrSessionResetInProgress) {
		t.Fatalf("expected ErrSessionResetInProgress, got %v", err)
	}
}

// --- CancelAgent ---

// TestCancelAgent_DeduplicatesConcurrentCalls covers the impatient-user case:
// the UI's cancel button has no in-flight disable, so users click it multiple
// times while the agent is still tearing down a slow turn (e.g. a Claude
// Monitor tool). Without dedupe each click reaches the lifecycle layer and
// emits its own "Turn cancelled by user" message; phantom turns are also
// lazily started to host those messages. We assert that only one cancel makes
// it through to agentManager.CancelAgent while one is already in flight.
func TestCancelAgent_DeduplicatesConcurrentCalls(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isAgentRunning:     true,
		cancelAgentBlock:   make(chan struct{}),
		cancelAgentEntered: make(chan struct{}, 1),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	// First call goes async and parks inside agentManager.CancelAgent.
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- svc.CancelAgent(context.Background(), "session1")
	}()

	// Wait for the first call to actually enter agentManager.CancelAgent so
	// the dedupe guard is set before the duplicate calls fire. Channel sync
	// (over sleep-based polling) is the project convention for tests that
	// don't depend on real subprocess timing.
	<-agentMgr.cancelAgentEntered

	// Fire several duplicates while the first is still parked. Each must be
	// short-circuited by the dedupe guard and return immediately.
	const duplicates = 5
	for i := 0; i < duplicates; i++ {
		if err := svc.CancelAgent(context.Background(), "session1"); err != nil {
			t.Fatalf("duplicate cancel %d returned error: %v", i, err)
		}
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 agentManager.CancelAgent call while first is in flight, got %d", got)
	}

	// Release the first call. After it returns, the guard clears and a fresh
	// cancel is allowed through.
	close(agentMgr.cancelAgentBlock)
	if err := <-firstDone; err != nil {
		t.Fatalf("first CancelAgent returned error: %v", err)
	}

	agentMgr.cancelAgentBlock = nil // unblock subsequent calls
	if err := svc.CancelAgent(context.Background(), "session1"); err != nil {
		t.Fatalf("post-release CancelAgent returned error: %v", err)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 2 {
		t.Fatalf("expected 2 agentManager.CancelAgent calls after release, got %d", got)
	}
}

// --- StartCreatedSession ---

func TestStartCreatedSession_WrongTask(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Session belongs to "task-other", not "task1"
	seedTaskAndSession(t, repo, "task-other", "session1", models.TaskSessionStateCreated)

	_, err := svc.StartCreatedSession(context.Background(), "task1", "session1", "profile1", "prompt", false, false, nil)
	if err == nil {
		t.Fatal("expected error when session does not belong to task")
	}
}

func TestStartCreatedSession_NotInCreatedState(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	_, err := svc.StartCreatedSession(context.Background(), "task1", "session1", "profile1", "prompt", false, false, nil)
	if err == nil {
		t.Fatal("expected error when session is not in CREATED state")
	}
}

// --- recordInitialMessage ---

// mockMessageCreator implements MessageCreator for testing.
// Only CreateUserMessage is tracked; all other methods are no-op stubs.
type mockMessageCreator struct {
	userMessages    []mockUserMessage
	sessionMessages []mockSessionMessage
}

type mockUserMessage struct {
	taskID, content, sessionID, turnID string
	metadata                           map[string]interface{}
}

type mockSessionMessage struct {
	taskID, content, sessionID, messageType, turnID string
	metadata                                        map[string]interface{}
	requestsInput                                   bool
}

func (m *mockMessageCreator) CreateUserMessage(_ context.Context, taskID, content, sessionID, turnID string, metadata map[string]interface{}) error {
	m.userMessages = append(m.userMessages, mockUserMessage{taskID, content, sessionID, turnID, metadata})
	return nil
}

func (m *mockMessageCreator) CreateAgentMessage(context.Context, string, string, string, string) error {
	return nil
}

func (m *mockMessageCreator) CreateToolCallMessage(context.Context, string, string, string, string, string, string, string, *streams.NormalizedPayload) error {
	return nil
}

func (m *mockMessageCreator) UpdateToolCallMessage(context.Context, string, string, string, string, string, string, string, string, string, *streams.NormalizedPayload) error {
	return nil
}

func (m *mockMessageCreator) CreateSessionMessage(_ context.Context, taskID, content, sessionID, messageType, turnID string, metadata map[string]interface{}, requestsInput bool) error {
	m.sessionMessages = append(m.sessionMessages, mockSessionMessage{
		taskID:        taskID,
		content:       content,
		sessionID:     sessionID,
		messageType:   messageType,
		turnID:        turnID,
		metadata:      metadata,
		requestsInput: requestsInput,
	})
	return nil
}

func (m *mockMessageCreator) CreatePermissionRequestMessage(context.Context, string, string, string, string, string, string, []map[string]interface{}, string, map[string]interface{}) (string, error) {
	return "", nil
}

func (m *mockMessageCreator) UpdatePermissionMessage(context.Context, string, string, string) error {
	return nil
}

func (m *mockMessageCreator) CreateAgentMessageStreaming(context.Context, string, string, string, string, string) error {
	return nil
}

func (m *mockMessageCreator) AppendAgentMessage(context.Context, string, string) error {
	return nil
}

func (m *mockMessageCreator) CreateThinkingMessageStreaming(context.Context, string, string, string, string, string) error {
	return nil
}

func (m *mockMessageCreator) AppendThinkingMessage(context.Context, string, string) error {
	return nil
}
func (m *mockMessageCreator) InvalidateModelCache(string) {}

func TestRecordInitialMessage_DoesNotChangeSessionState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	svc.recordInitialMessage(ctx, "task1", "session1", "hello world", false, nil)

	// Session state must remain STARTING — recordInitialMessage should not modify state.
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.State != models.TaskSessionStateStarting {
		t.Fatalf("expected session state %q, got %q", models.TaskSessionStateStarting, session.State)
	}

	// User message should have been created.
	if len(mc.userMessages) != 1 {
		t.Fatalf("expected 1 user message, got %d", len(mc.userMessages))
	}
	if mc.userMessages[0].content != "hello world" {
		t.Fatalf("expected message content %q, got %q", "hello world", mc.userMessages[0].content)
	}
}

func TestPostLaunchCreated_SkipMessage_DoesNotChangeSessionState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	svc.postLaunchCreated(ctx, "task1", "session1", "prompt", true, false, nil)

	// Session state must remain STARTING when skipMessage=true.
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.State != models.TaskSessionStateStarting {
		t.Fatalf("expected session state %q, got %q", models.TaskSessionStateStarting, session.State)
	}
}

func TestPostLaunchCreated_WithMessage_DoesNotChangeSessionState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	svc.postLaunchCreated(ctx, "task1", "session1", "hello", false, false, nil)

	// Session state must remain STARTING — postLaunchCreated delegates to
	// recordInitialMessage which only creates the message.
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.State != models.TaskSessionStateStarting {
		t.Fatalf("expected session state %q, got %q", models.TaskSessionStateStarting, session.State)
	}

	if len(mc.userMessages) != 1 {
		t.Fatalf("expected 1 user message, got %d", len(mc.userMessages))
	}
}

func TestPostLaunchCreated_PlanMode_SetsMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	mc := &mockMessageCreator{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = mc

	svc.postLaunchCreated(ctx, "task1", "session1", "plan this", false, true, nil)

	// User message should have plan_mode metadata.
	if len(mc.userMessages) != 1 {
		t.Fatalf("expected 1 user message, got %d", len(mc.userMessages))
	}
	if mc.userMessages[0].metadata["plan_mode"] != true {
		t.Fatalf("expected plan_mode=true in metadata, got %v", mc.userMessages[0].metadata)
	}

	// Session metadata should contain plan_mode.
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if session.Metadata == nil {
		t.Fatal("expected session metadata to be set")
	}
	if session.Metadata["plan_mode"] != true {
		t.Fatalf("expected plan_mode=true in session metadata, got %v", session.Metadata["plan_mode"])
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

func TestResumeTaskSession_ArchivedTaskSkipsFailedState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	// Archive the task after seeding
	if err := repo.ArchiveTask(ctx, "task1"); err != nil {
		t.Fatalf("failed to archive task: %v", err)
	}

	// Insert executor running record so we pass the "not resumable" check
	now := time.Now().UTC()
	_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "er1", SessionID: "session1", TaskID: "task1",
		CreatedAt: now, UpdatedAt: now,
	})

	_, err := svc.ResumeTaskSession(ctx, "task1", "session1")
	if !errors.Is(err, executor.ErrTaskArchived) {
		t.Fatalf("expected ErrTaskArchived, got: %v", err)
	}

	// Task state should NOT have been updated to FAILED
	if _, ok := taskRepo.updatedStates["task1"]; ok {
		t.Error("task state should not be updated when task is archived")
	}
}

func TestResumeTaskSession_ArchivedDuringLaunch(t *testing.T) {
	// Simulates the race: task is NOT archived when the executor checks,
	// but LaunchAgent fails (archive's async cleanup killed the agent),
	// and by the time the error path re-reads the task it IS archived.
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			// Simulate archive completing while launch is in progress:
			// archive the task, then fail the launch (as if async cleanup killed the process).
			_ = repo.ArchiveTask(ctx, "task1")
			return nil, fmt.Errorf("connection refused")
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	now := time.Now().UTC()
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	// Set agent profile ID so the executor doesn't reject the session early
	session, _ := repo.GetTaskSession(ctx, "session1")
	session.AgentProfileID = "profile-1"
	_ = repo.UpdateTaskSession(ctx, session)

	_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "er1", SessionID: "session1", TaskID: "task1",
		CreatedAt: now, UpdatedAt: now,
	})

	_, err := svc.ResumeTaskSession(ctx, "task1", "session1")
	if !errors.Is(err, executor.ErrTaskArchived) {
		t.Fatalf("expected ErrTaskArchived, got: %v", err)
	}

	// Task state should NOT have been updated to FAILED
	if _, ok := taskRepo.updatedStates["task1"]; ok {
		t.Error("task state should not be updated when task is archived during launch")
	}
}

// TestResumeTaskSession_FailedKeepsResumeToken verifies that resuming a FAILED
// session preserves the ACP resume token so the relaunched agent restores the
// prior conversation via ACP session/load (for native-resume agents).
// Regression test for the "Resume blocked by stale state" bug where FAILED sessions
// couldn't be restarted at all; the fix also changes policy to keep the token
// (previously it was cleared to force a fresh agent).
func TestResumeTaskSession_FailedKeepsResumeToken(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()

	// Agent launch succeeds so the resume path does not unwind and mark the task
	// FAILED, which would exercise a separate state-mutation code path.
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{
				AgentExecutionID: "exec-new",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateFailed)
	session, _ := repo.GetTaskSession(ctx, "session1")
	session.AgentProfileID = "profile-1"
	_ = repo.UpdateTaskSession(ctx, session)

	now := time.Now().UTC()
	_ = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "er1", SessionID: "session1", TaskID: "task1",
		ResumeToken: "acp-session-xyz",
		Resumable:   true,
		CreatedAt:   now, UpdatedAt: now,
	})

	if _, err := svc.ResumeTaskSession(ctx, "task1", "session1"); err != nil {
		t.Fatalf("ResumeTaskSession on FAILED session returned: %v", err)
	}

	er, err := repo.GetExecutorRunningBySessionID(ctx, "session1")
	if err != nil || er == nil {
		t.Fatalf("ExecutorRunning lookup failed: %v (nil=%v)", err, er == nil)
	}
	if er.ResumeToken != "acp-session-xyz" {
		t.Errorf("expected resume token to be preserved on FAILED resume, got %q", er.ResumeToken)
	}
}

// --- CompleteTask ---

func TestCompleteTask_UpdatesTaskState(t *testing.T) {
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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

	t.Run("isAgentAlreadyRunningError", func(t *testing.T) {
		tests := []struct {
			name string
			err  error
			want bool
		}{
			{"nil error", nil, false},
			{"unrelated error", errors.New("something else"), false},
			{"lifecycle manager error", fmt.Errorf("session %q already has an agent running (execution: %s)", "s1", "exec-1"), true},
			{"wrapped error", fmt.Errorf("failed to resume session: %w", fmt.Errorf("session %q already has an agent running (execution: %s)", "s1", "exec-1")), true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := isAgentAlreadyRunningError(tc.err); got != tc.want {
					t.Errorf("isAgentAlreadyRunningError(%v) = %v, want %v", tc.err, got, tc.want)
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

// --- GetTaskSessionStatus ---

func TestGetTaskSessionStatus_HealsStuckStartingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateStarting)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-active"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-active")
	session.UpdatedAt = time.Now().UTC()
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er1",
		SessionID:        "session1",
		TaskID:           "task1",
		Status:           "ready",
		Resumable:        true,
		AgentExecutionID: "exec-active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.State != string(models.TaskSessionStateWaitingForInput) {
		t.Fatalf("expected response state %q, got %q", models.TaskSessionStateWaitingForInput, resp.State)
	}

	updated, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected persisted session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
	}
	if state, ok := taskRepo.updatedStates["task1"]; !ok || state != v1.TaskStateReview {
		t.Fatalf("expected task state %q, got %q (ok=%v)", v1.TaskStateReview, state, ok)
	}
}

// TestGetTaskSessionStatus_DoesNotHealOnMismatchedExecution was removed.
// The pre-refactor heal check skipped healing when session.AgentExecutionID and
// running.AgentExecutionID disagreed — a band-aid for the very divergence bug
// this PR fixes structurally. With executors_running as the single source of
// truth (lifecycle-owned, persisted in lockstep with executionStore.Add), the
// mismatch this test simulated cannot occur, and the band-aid was removed
// (see shouldHealStuckStartingSession in task_operations.go).

func TestGetTaskSessionStatus_UsesTaskEnvironmentBranchForDocker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.TaskEnvironmentID = "env1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:             "env1",
		TaskID:         "task1",
		ExecutorType:   string(models.ExecutorTypeLocalDocker),
		WorktreePath:   "/workspace",
		WorktreeBranch: "feature/test-task-abc",
		WorkspacePath:  "/workspace",
		Status:         models.TaskEnvironmentStatusReady,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}

	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.WorktreeBranch == nil || *resp.WorktreeBranch != "feature/test-task-abc" {
		t.Fatalf("worktree_branch = %v, want feature/test-task-abc", resp.WorktreeBranch)
	}
	if resp.WorktreePath == nil || *resp.WorktreePath != "/workspace" {
		t.Fatalf("worktree_path = %v, want /workspace", resp.WorktreePath)
	}
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

// --- ensureSessionRunning: prepared workspace ---

func TestEnsureSessionRunning_PreparedWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	// Seed task and session in CREATED state (workspace prepared, agent not started)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)

	// Set AgentExecutionID to simulate a prepared workspace
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	session.AgentExecutionID = "exec-prepare-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-prepare-1")
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	// Create a wrapped mock agent manager that transitions the session to WAITING_FOR_INPUT
	// when StartAgentProcess is called (simulating the agent starting successfully).
	startAgentProcessCalled := false
	wrappedMgr := &sessionUpdatingAgentManager{
		mockAgentManager: &mockAgentManager{
			isAgentRunning: false,
			// Return the execution ID so the existing-workspace path proceeds
			getExecutionIDForSessionFunc: func(_ context.Context, sid string) (string, error) {
				if sid == "session1" {
					return "exec-prepare-1", nil
				}
				return "", fmt.Errorf("no execution found")
			},
		},
		repo:          repo,
		sessionID:     "session1",
		taskID:        "task1",
		onStartCalled: &startAgentProcessCalled,
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{
		ID:          "task1",
		Title:       "Test Task",
		Description: "desc",
		State:       v1.TaskStateInProgress,
	}

	log := testLogger()
	exec := executor.NewExecutor(wrappedMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.DefaultSchedulerConfig())

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, wrappedMgr)
	svc.executor = exec
	svc.scheduler = sched

	// Re-load session for the call
	session, err = repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}

	err = svc.ensureSessionRunning(ctx, "session1", session)
	if err != nil {
		t.Fatalf("ensureSessionRunning failed: %v", err)
	}

	if !startAgentProcessCalled {
		t.Fatal("expected StartAgentProcess to be called (prepared workspace path)")
	}

	// Verify the session transitioned through STARTING
	updated, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session state %q, got %q", models.TaskSessionStateWaitingForInput, updated.State)
	}
}

func TestEnsureSessionRunning_WaitingForInputUsesResumePath(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	// Session in WAITING_FOR_INPUT without executor running record → resume path fails gracefully
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = exec

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	// Should fail because there is no executor running record (resume path)
	err = svc.ensureSessionRunning(ctx, "session1", session)
	if err == nil {
		t.Fatal("expected error for WAITING_FOR_INPUT session without executor record")
	}
	// Verify it took the resume path (error mentions "not resumable")
	if !strings.Contains(err.Error(), "not resumable") {
		t.Fatalf("expected 'not resumable' error from resume path, got: %v", err)
	}
}

func TestEnsureSessionRunning_CreatedWithoutExecutionUsesResumePath(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	// Session in CREATED state WITHOUT AgentExecutionID → resume path (not prepared workspace)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCreated)

	agentMgr := &mockAgentManager{isAgentRunning: false}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = exec

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	// AgentExecutionID is empty → should NOT take prepared workspace path
	// Should fail with "not resumable" because no executor running record
	err = svc.ensureSessionRunning(ctx, "session1", session)
	if err == nil {
		t.Fatal("expected error for CREATED session without executor record")
	}
	if !strings.Contains(err.Error(), "not resumable") {
		t.Fatalf("expected 'not resumable' error from resume path, got: %v", err)
	}
}

// --- canRestoreWorkspace ---

func TestCanRestoreWorkspace(t *testing.T) {
	tests := []struct {
		name string
		resp *dto.TaskSessionStatusResponse
		want bool
	}{
		{
			name: "nil response",
			resp: nil,
			want: false,
		},
		{
			name: "nil worktree path",
			resp: &dto.TaskSessionStatusResponse{},
			want: false,
		},
		{
			name: "empty worktree path",
			resp: &dto.TaskSessionStatusResponse{WorktreePath: strPtr("")},
			want: false,
		},
		{
			name: "valid worktree path",
			resp: &dto.TaskSessionStatusResponse{WorktreePath: strPtr("/tmp/worktrees/session1")},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canRestoreWorkspace(tc.resp); got != tc.want {
				t.Errorf("canRestoreWorkspace() = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- GetTaskSessionStatus: NeedsWorkspaceRestore ---

func TestGetTaskSessionStatus_NeedsWorkspaceRestore_TerminalWithWorktree(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCompleted)

	// Add worktree to session
	now := time.Now().UTC()
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "wt1",
		SessionID:      "session1",
		WorktreeID:     "wid1",
		RepositoryID:   "repo1",
		WorktreePath:   "/tmp/worktrees/session1",
		WorktreeBranch: "feature/test",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if !resp.NeedsWorkspaceRestore {
		t.Fatal("expected NeedsWorkspaceRestore=true for terminal session with worktree")
	}
	if resp.NeedsResume {
		t.Fatal("expected NeedsResume=false for terminal session")
	}
}

func TestGetTaskSessionStatus_NeedsWorkspaceRestore_TerminalWithoutWorktree(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)

	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateCompleted)

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	resp, err := svc.GetTaskSessionStatus(ctx, "task1", "session1")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus returned error: %v", err)
	}
	if resp.NeedsWorkspaceRestore {
		t.Fatal("expected NeedsWorkspaceRestore=false for terminal session without worktree")
	}
}

// sessionUpdatingAgentManager wraps mockAgentManager to update session state
// when StartAgentProcess is called, simulating the agent initialization flow.
type sessionUpdatingAgentManager struct {
	*mockAgentManager
	repo          *sqliterepo.Repository
	sessionID     string
	taskID        string
	onStartCalled *bool
}

func (m *sessionUpdatingAgentManager) StartAgentProcess(_ context.Context, _ string) error {
	*m.onStartCalled = true
	// Simulate the agent starting by transitioning session to WAITING_FOR_INPUT
	ctx := context.Background()
	sess, err := m.repo.GetTaskSession(ctx, m.sessionID)
	if err == nil && sess != nil {
		sess.State = models.TaskSessionStateWaitingForInput
		sess.UpdatedAt = time.Now().UTC()
		_ = m.repo.UpdateTaskSession(ctx, sess)
	}
	return nil
}
