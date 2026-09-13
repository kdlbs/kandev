package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type workflowResetFailureBarrierRepo struct {
	repoStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type blockingWorkflowResetAgentManager struct {
	*mockAgentManager
	resetStarted chan struct{}
	resetRelease chan struct{}
	resetOnce    sync.Once
}

func (m *blockingWorkflowResetAgentManager) ResetAgentContext(ctx context.Context, _ string) error {
	m.resetOnce.Do(func() { close(m.resetStarted) })
	select {
	case <-m.resetRelease:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *workflowResetFailureBarrierRepo) SetSessionMetadataKey(
	ctx context.Context, sessionID, key string, value interface{},
) error {
	if key == models.SessionMetaKeyLastAgentError {
		r.once.Do(func() { close(r.entered) })
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.repoStore.SetSessionMetadataKey(ctx, sessionID, key, value)
}

func TestProcessOnEnter_ResetFailurePersistsNotice(t *testing.T) {
	tests := []struct {
		name                string
		newAgentManager     func() *mockAgentManager
		prepare             func(*Service, *models.TaskSession)
		expectedExecutionID string
	}{
		{
			name: "provider reset failure",
			newAgentManager: func() *mockAgentManager {
				return &mockAgentManager{
					restartProcessErr: errors.New("provider reset failed"),
				}
			},
			expectedExecutionID: "exec-reset",
		},
		{
			name: "active cancellation escalation",
			newAgentManager: func() *mockAgentManager {
				return &mockAgentManager{cancelAgentErr: lifecycle.ErrCancelEscalated}
			},
			prepare: func(svc *Service, session *models.TaskSession) {
				session.AgentExecutionID = "exec-reset"
				svc.activeTurns.Store(session.ID, "turn-reset")
			},
			expectedExecutionID: "exec-reset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			session, err := repo.GetTaskSession(context.Background(), "s1")
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-reset")
			// processOnEnter receives this object from the transition handler. Keep
			// an intentionally stale copy here to prove the final event reloads the
			// metadata written by persistLastAgentError.
			session.Metadata = map[string]interface{}{
				models.SessionMetaKeyLastAgentError: models.LastAgentError{Message: "stale reset notice"},
			}

			eventBus := &mockEventBus{}
			agentManager := tt.newAgentManager()
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
			svc.eventBus = eventBus
			messages := &mockMessageCreator{}
			svc.messageCreator = messages
			if tt.prepare != nil {
				tt.prepare(svc, session)
			}

			svc.processOnEnter(context.Background(), "t1", session, workflowResetFailureTestStep(), "review task", 0, nil)
			assertResetFailureFeedback(t, repo, eventBus, agentManager, messages, tt.expectedExecutionID)
		})
	}
}

func TestProcessOnEnter_ResetFailureIgnoresCallerCancellation(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-reset")

	eventBus := &mockEventBus{}
	agentManager := &mockAgentManager{restartProcessErr: errors.New("provider reset failed")}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.eventBus = eventBus
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	step := workflowResetFailureTestStep()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.processOnEnter(ctx, "t1", session, step, "review task", 0, nil)

	assertResetFailureFeedback(t, repo, eventBus, agentManager, messages, "exec-reset")
}

func TestProcessOnEnter_ResetFailureLogsPersistenceFailure(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-reset")

	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	eventBus := &mockEventBus{}
	agentManager := &mockAgentManager{restartProcessErr: errors.New("provider reset failed")}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.logger = log
	svc.repo = failSetSessionMetadataRepo{repoStore: repo}
	svc.eventBus = eventBus
	svc.messageCreator = &mockMessageCreator{}

	svc.processOnEnter(context.Background(), "t1", session, workflowResetFailureTestStep(), "review task", 0, nil)

	if logs.FilterMessage("failed to persist workflow context reset failure").Len() == 0 {
		t.Fatal("expected a persistence failure log")
	}
	stored, err := repo.GetTaskSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if stored.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want WAITING_FOR_INPUT after persistence failure", stored.State)
	}
	for _, published := range eventBus.published() {
		if published.Subject == events.TaskSessionErrorChanged {
			t.Fatal("persistence failure must not publish a false active error event")
		}
	}
}

func TestProcessOnEnter_ResetFailureSettlesStateAfterPersistenceBudgetExpires(t *testing.T) {
	previousTimeout := workflowResetFailureCleanupTimeout
	workflowResetFailureCleanupTimeout = 25 * time.Millisecond
	t.Cleanup(func() { workflowResetFailureCleanupTimeout = previousTimeout })

	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedSessionRecord, err := repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	seedExecutorRunning(t, repo, seedSessionRecord.ID, seedSessionRecord.TaskID, "exec-reset")

	barrier := &workflowResetFailureBarrierRepo{
		repoStore: repo,
		entered:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	manager := &mockAgentManager{restartProcessErr: errors.New("provider reset failed")}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.repo = barrier
	svc.eventBus = &mockEventBus{}
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	resetDone := make(chan struct{})
	go func() {
		svc.processOnEnter(
			context.Background(), "t1", seedSessionRecord, workflowResetFailureTestStep(),
			"review task", 0, nil,
		)
		close(resetDone)
	}()
	waitForWorkflowResetBarrier(t, barrier.entered)

	select {
	case <-resetDone:
	case <-time.After(time.Second):
		t.Fatal("workflow reset failure did not settle after metadata persistence timed out")
	}

	stored, err := repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, stored.State)
}

func TestProcessOnEnter_SuccessfulResetDispatchesAutoStartPrompt(t *testing.T) {
	svc, repo, manager, session := newActiveResetTestService(t)
	manager.isAgentRunning = true
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.processOnEnter(
		context.Background(), "task1", session, workflowResetFailureTestStep(),
		"review task", 0, nil,
	)

	requireResetEvents(t, manager.events, "cancel", "reset")
	if len(manager.capturedPrompts) != 1 {
		t.Fatalf("successful workflow entry dispatched %d prompts, want 1", len(manager.capturedPrompts))
	}
	if !strings.Contains(manager.capturedPrompts[0], "distinct reset failure workflow prompt") {
		t.Fatalf("dispatched prompt = %q, want the distinct step prompt", manager.capturedPrompts[0])
	}
	if len(messages.userMessages) != 1 {
		t.Fatalf("successful workflow entry persisted %d user messages, want 1", len(messages.userMessages))
	}
}

func TestProcessOnEnter_ResetFailureSettlementOwnsSuccessorAdmission(t *testing.T) {
	t.Run("successor admission waits for failure settlement", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedExecutorRunning(t, repo, "s1", "t1", "exec-reset")
		session, err := repo.GetTaskSession(context.Background(), "s1")
		require.NoError(t, err)
		session.State = models.TaskSessionStateWaitingForInput
		require.NoError(t, repo.UpdateTaskSession(context.Background(), session))
		session, err = repo.GetTaskSession(context.Background(), "s1")
		require.NoError(t, err)

		barrier := &workflowResetFailureBarrierRepo{
			repoStore: repo,
			entered:   make(chan struct{}),
			release:   make(chan struct{}),
		}
		manager := &mockAgentManager{restartProcessErr: errors.New("provider reset failed")}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
		svc.repo = barrier
		svc.eventBus = &mockEventBus{}
		svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

		resetDone := make(chan struct{})
		go func() {
			svc.processOnEnter(context.Background(), "t1", session, workflowResetFailureTestStep(), "review task", 0, nil)
			close(resetDone)
		}()
		waitForWorkflowResetBarrier(t, barrier.entered)

		successorAdmitted := make(chan struct{})
		successorDone := make(chan error, 1)
		go func() {
			lock, release := svc.acquireCancelInFlightGuard("s1")
			lock.Lock()
			close(successorAdmitted)
			successorDone <- repo.UpdateTaskSessionState(
				context.Background(), "s1", models.TaskSessionStateRunning, "successor turn",
			)
			lock.Unlock()
			release()
		}()
		assertWorkflowResetChannelBlocked(t, successorAdmitted, "successor admission")

		close(barrier.release)
		<-resetDone
		require.NoError(t, <-successorDone)
		stored, err := repo.GetTaskSession(context.Background(), "s1")
		require.NoError(t, err)
		require.Equal(t, models.TaskSessionStateRunning, stored.State)
		if _, ok := models.LoadLastAgentError(stored.Metadata); !ok {
			t.Fatal("failure settlement lost the durable error before successor admission")
		}
	})

	t.Run("deletion waits for failure settlement", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedExecutorRunning(t, repo, "s1", "t1", "exec-reset")
		session, err := repo.GetTaskSession(context.Background(), "s1")
		require.NoError(t, err)
		session.State = models.TaskSessionStateWaitingForInput
		require.NoError(t, repo.UpdateTaskSession(context.Background(), session))
		session, err = repo.GetTaskSession(context.Background(), "s1")
		require.NoError(t, err)

		barrier := &workflowResetFailureBarrierRepo{
			repoStore: repo,
			entered:   make(chan struct{}),
			release:   make(chan struct{}),
		}
		manager := &mockAgentManager{restartProcessErr: errors.New("provider reset failed")}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
		svc.repo = barrier
		svc.eventBus = &mockEventBus{}
		svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

		resetDone := make(chan struct{})
		go func() {
			svc.processOnEnter(context.Background(), "t1", session, workflowResetFailureTestStep(), "review task", 0, nil)
			close(resetDone)
		}()
		waitForWorkflowResetBarrier(t, barrier.entered)

		deleteDone := make(chan error, 1)
		go func() { deleteDone <- svc.DeleteSession(context.Background(), "s1") }()
		assertWorkflowResetChannelBlocked(t, deleteDone, "session deletion")

		close(barrier.release)
		<-resetDone
		require.NoError(t, <-deleteDone)
		_, err = repo.GetTaskSession(context.Background(), "s1")
		require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	})
}

func TestResetAgentContext_BlocksDeletionWhileProviderCallIsInFlight(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-reset")
	session, err := repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(context.Background(), session))

	manager := &blockingWorkflowResetAgentManager{
		mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo},
		resetStarted:     make(chan struct{}),
		resetRelease:     make(chan struct{}),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	resetDone := make(chan error, 1)
	go func() {
		_, resetErr := svc.resetAgentContextWithError(
			context.Background(), "t1", session, "Review Step",
		)
		resetDone <- resetErr
	}()
	waitForWorkflowResetBarrier(t, manager.resetStarted)

	deleteErr := svc.DeleteSession(context.Background(), "s1")
	require.ErrorIs(t, deleteErr, ErrSessionResetInProgress)
	_, err = repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err, "deletion must not remove a session during provider reset")

	close(manager.resetRelease)
	require.NoError(t, <-resetDone)
	require.NoError(t, svc.DeleteSession(context.Background(), "s1"))
	_, err = repo.GetTaskSession(context.Background(), "s1")
	require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
}

func waitForWorkflowResetBarrier(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for workflow reset failure persistence barrier")
	}
}

func assertWorkflowResetChannelBlocked[T any](t *testing.T, channel <-chan T, operation string) {
	t.Helper()
	select {
	case value := <-channel:
		t.Fatalf("%s completed before reset failure settlement: %#v", operation, value)
	case <-time.After(50 * time.Millisecond):
	}
}

func workflowResetFailureTestStep() *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Review Step",
		Prompt: "distinct reset failure workflow prompt",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
			{Type: wfmodels.OnEnterAutoStartAgent},
		}},
	}
}

func assertResetFailureFeedback(
	t *testing.T,
	repo *sqliterepo.Repository,
	eventBus *mockEventBus,
	agentManager *mockAgentManager,
	messages *mockMessageCreator,
	expectedExecutionID string,
) {
	t.Helper()
	ctx := context.Background()
	stored, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(stored.Metadata)
	if !ok {
		t.Fatal("expected a durable workflow reset error notice")
	}
	message := strings.ToLower(lastError.Message)
	if !strings.Contains(message, "context reset") || !strings.Contains(message, "step prompt") {
		t.Fatalf("reset error message = %q, want reset and prompt details", lastError.Message)
	}
	if lastError.AgentExecutionID != expectedExecutionID {
		t.Fatalf("error execution ID = %q, want %q", lastError.AgentExecutionID, expectedExecutionID)
	}
	if lastError.Code != workflowResetFailureCode {
		t.Fatalf("error code = %q, want %q", lastError.Code, workflowResetFailureCode)
	}
	if strings.Contains(lastError.Details, "stale reset notice") {
		t.Fatalf("failure details retained stale snapshot data: %q", lastError.Details)
	}

	if len(agentManager.capturedPrompts) != 0 {
		t.Fatalf("failed workflow entry dispatched %d prompts", len(agentManager.capturedPrompts))
	}
	if len(messages.userMessages) != 0 {
		t.Fatalf("failed workflow entry persisted %d user messages", len(messages.userMessages))
	}

	var errorEvent, finalWaitingEvent *publishedEvent
	publishedEvents := eventBus.published()
	for i := range publishedEvents {
		published := publishedEvents[i]
		switch published.Subject {
		case events.TaskSessionErrorChanged:
			errorEvent = &published
		case events.TaskSessionStateChanged:
			data, _ := published.Event.Data.(map[string]interface{})
			if data["workflow_step_id"] == "step2" {
				finalWaitingEvent = &published
			}
		}
	}
	if errorEvent == nil {
		t.Fatal("expected a published last-agent-error event")
	}
	errorData, _ := errorEvent.Event.Data.(map[string]interface{})
	if active, _ := errorData["active"].(bool); !active {
		t.Fatalf("last-agent-error event data = %#v, want active=true", errorData)
	}
	if errorData["code"] != workflowResetFailureCode {
		t.Fatalf("last-agent-error event code = %#v, want %q", errorData["code"], workflowResetFailureCode)
	}
	if finalWaitingEvent == nil {
		t.Fatal("expected a final waiting-state event for the destination step")
	}
	waitingData, _ := finalWaitingEvent.Event.Data.(map[string]interface{})
	metadata, ok := waitingData["session_metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("waiting event metadata = %#v, want fresh session metadata", waitingData["session_metadata"])
	}
	waitingError, ok := models.LoadLastAgentError(metadata)
	if !ok || waitingError.Message != lastError.Message {
		t.Fatalf("waiting event omitted the persisted reset error: %#v", metadata)
	}
}
