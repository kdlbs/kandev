package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	agentexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestAgentTurnFailedSettlesDurableErrorWithoutStoppingRuntime(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc
	svc.beginPromptAttempt("s1", "execution-1", 7, false)

	data := retainedTurnFailureData()
	svc.handleAgentTurnFailed(ctx, data)

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Len(t, mc.sessionMessages, 1)
	message := mc.sessionMessages[0]
	require.Equal(t, "turn-1", message.turnID)
	require.Equal(t, "error", message.metadata["variant"])
	require.Equal(t, "turn", message.metadata["failure_scope"])
	require.Equal(t, true, message.metadata["runtime_retained"])
	require.Equal(t, "execution-1", message.metadata["execution_id"])
	require.Equal(t, uint64(7), message.metadata["prompt_generation"])
	require.NotEqual(t, true, message.metadata["recovery_actions"])
	require.Equal(t, "refused", message.metadata["recovery_disposition"])
	require.Equal(t, 0, message.metadata["attempts_started"])

	svc.handleAgentTurnFailed(ctx, data)
	require.Len(t, mc.sessionMessages, 1, "duplicate completion must not create another turn error")
	agentMgr.mu.Lock()
	require.Empty(t, agentMgr.stopAgentWithReasonArgs, "a retained turn failure must keep the process alive")
	agentMgr.mu.Unlock()
}

func TestRetainedFailureCompleteStreamDefersSettlementToTurnOwner(t *testing.T) {
	ctx := context.Background()
	svc, messageCreator := newTransientTestService(t)
	svc.beginPromptAttempt("s1", "execution-1", 7, false)
	svc.observePromptAttempt("s1", "execution-1", 7, true, true)

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: "s1", ExecutionID: "execution-1", OwnerKind: lifecycle.ExecutionOwnerTask,
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventComplete, TurnID: "turn-1", PromptGeneration: 7,
			Error:                    "Selected model is at capacity. Please try a different model.",
			PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
			Data:                     map[string]interface{}{"is_error": true},
		},
	})

	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State,
		"the stream completion must not settle a retained provider failure")
	require.True(t, svc.currentRetainedTurnFailureAttempt("s1", "execution-1", 7),
		"the retained turn owner still needs the live execution and generation evidence")
	require.Empty(t, messageCreator.sessionMessages)

	svc.handleAgentTurnFailed(ctx, retainedTurnFailureData())

	session, err = svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Len(t, messageCreator.sessionMessages, 1,
		"the retained turn-failure owner must persist the sole durable error outcome")
}

func TestAgentTurnFailureStorageErrorKeepsSessionAdmissionClosed(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	mc := &mockMessageCreator{sessionMessageErr: errors.New("message storage unavailable")}
	svc.messageCreator = mc
	svc.beginPromptAttempt("s1", "execution-1", 7, false)

	svc.handleAgentTurnFailed(ctx, retainedTurnFailureData())

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	_, ok := svc.promptAttemptForSession("s1")
	require.True(t, ok, "the failed settlement must retain its ownership fence")
}

func TestSynchronousTurnFailureWatcherDoesNotDeadlockWithExplicitCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, messageCreator := newTransientTestService(t)
	mgr := svc.agentManager.(*mockAgentManager)
	var promptLifecycleMu sync.Mutex
	ackManager := &retainedPromptAckAgentManager{
		mockAgentManager:  mgr,
		promptLifecycleMu: &promptLifecycleMu,
		pending:           true,
	}
	svc.agentManager = ackManager
	mgr.currentPromptExecutionID = "execution-1"
	mgr.currentPromptGeneration.Store(7)
	seedExecutorRunning(t, svc.repo.(*sqliterepo.Repository), "s1", "t1", "execution-1")
	armTransientPromptEvidence(svc)

	promptLifecycleMu.Lock()
	var releasePromptLifecycleOnce sync.Once
	releasePromptLifecycle := func() {
		releasePromptLifecycleOnce.Do(promptLifecycleMu.Unlock)
	}
	generationRead := make(chan struct{})
	var readOnce sync.Once
	mgr.getPromptGenerationForSessionFunc = func(context.Context, string) (uint64, error) {
		readOnce.Do(func() { close(generationRead) })
		promptLifecycleMu.Lock()
		defer promptLifecycleMu.Unlock()
		return mgr.currentPromptGeneration.Load(), nil
	}
	cancelRPCStarted := make(chan struct{})
	releaseCancelRPC := make(chan struct{})
	mgr.cancelAgentFunc = func(context.Context, string) error {
		close(cancelRPCStarted)
		<-releaseCancelRPC
		return nil
	}

	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(func() { eventBus.Close() })
	handlerEntered := make(chan struct{})
	handlerDone := make(chan struct{})
	var handlerEnteredOnce sync.Once
	var handlerDoneOnce sync.Once
	watch := watcher.NewWatcher(eventBus, watcher.EventHandlers{
		OnAgentTurnFailed: func(handlerCtx context.Context, data watcher.AgentEventData) {
			handlerEnteredOnce.Do(func() { close(handlerEntered) })
			svc.handleAgentTurnFailed(handlerCtx, data)
			handlerDoneOnce.Do(func() { close(handlerDone) })
		},
	}, "turn-failure-cancel-race", testLogger())
	require.NoError(t, watch.Start(ctx))
	t.Cleanup(func() { require.NoError(t, watch.Stop()) })
	t.Cleanup(releasePromptLifecycle)

	cancelDone := make(chan error, 1)
	go func() {
		cancelDone <- svc.CancelAgent(ctx, "s1")
	}()
	select {
	case <-generationRead:
	case <-time.After(time.Second):
		t.Fatal("explicit cancellation did not reach prompt identity capture")
	}

	publishDone := make(chan error, 1)
	go func() {
		payload := lifecycle.AgentEventPayload{
			TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", OwnerKind: lifecycle.ExecutionOwnerTask,
			AgentProfileID: "profile-codex", AgentID: "codex-acp", TurnID: "turn-1", PromptGeneration: 7,
			ErrorMessage:             "Selected model is at capacity. Please try a different model.",
			PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		}
		publishDone <- eventBus.Publish(ctx, events.AgentTurnFailed,
			bus.NewEvent(events.AgentTurnFailed, "lifecycle-manager", payload))
	}()
	select {
	case <-handlerEntered:
	case <-time.After(time.Second):
		t.Fatal("synchronous watcher did not enter the real retained turn-failure handler")
	}

	releasePromptLifecycle()
	select {
	case <-cancelRPCStarted:
	case <-time.After(time.Second):
		t.Fatal("explicit cancel did not proceed after prompt identity capture")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("retained turn-failure callback did not finish while cancellation RPC was in flight")
	}
	select {
	case err := <-publishDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("synchronous event publication did not settle")
	}
	close(releaseCancelRPC)
	select {
	case err := <-cancelDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("explicit cancellation did not finish")
	}
	ackManager.mu.Lock()
	require.Equal(t, []retainedPromptAck{{executionID: "execution-1", generation: 7}}, ackManager.acks,
		"the cancellation owner must settle the exact retained failure generation that its handler declined")
	ackManager.mu.Unlock()

	retainedCount := 0
	for _, message := range messageCreator.sessionMessages {
		if message.metadata["runtime_retained"] == true {
			retainedCount++
		}
	}
	require.Zero(t, retainedCount, "the explicit cancellation owner wins the in-flight turn settlement")

	repo := svc.repo.(*sqliterepo.Repository)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateRunning, ""))
	mgr.currentPromptGeneration.Store(8)
	svc.beginPromptAttempt("s1", "execution-1", 8, false)
	payload := lifecycle.AgentEventPayload{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", OwnerKind: lifecycle.ExecutionOwnerTask,
		AgentProfileID: "profile-codex", AgentID: "codex-acp", TurnID: "turn-1", PromptGeneration: 7,
		ErrorMessage:             "Selected model is at capacity. Please try a different model.",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
	}
	require.NoError(t, eventBus.Publish(ctx, events.AgentTurnFailed,
		bus.NewEvent(events.AgentTurnFailed, "lifecycle-manager", payload)))
	retainedCount = 0
	for _, message := range messageCreator.sessionMessages {
		if message.metadata["runtime_retained"] == true {
			retainedCount++
		}
	}
	require.Zero(t, retainedCount, "a stale completion cannot mutate successor prompt generation")
	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State,
		"a stale completion cannot settle the successor session")
	attempt, ok := svc.promptAttemptForSession("s1")
	require.True(t, ok)
	if ok {
		require.Equal(t, uint64(8), attempt.promptGeneration,
			"the successor prompt ownership must remain current")
	}
}

type retainedPromptAck struct {
	executionID string
	generation  uint64
}

type retainedPromptAckAgentManager struct {
	*mockAgentManager
	promptLifecycleMu *sync.Mutex
	mu                sync.Mutex
	acks              []retainedPromptAck
	pending           bool
}

func (m *retainedPromptAckAgentManager) AcknowledgeRetainedPromptFailure(executionID string, generation uint64) bool {
	m.promptLifecycleMu.Lock()
	defer m.promptLifecycleMu.Unlock()
	if executionID != m.currentPromptExecutionID || generation != m.currentPromptGeneration.Load() {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.pending {
		return false
	}
	m.pending = false
	m.acks = append(m.acks, retainedPromptAck{executionID: executionID, generation: generation})
	return true
}

func TestHandlePromptErrorDoesNotSettleRetainedTurnTwice(t *testing.T) {
	ctx := context.Background()
	svc, mc := newTransientTestService(t)
	err := svc.handlePromptError(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput,
		&lifecycle.RetainedPromptFailureError{
			Message:     "Selected model is at capacity.",
			Disposition: streams.PromptFailureDispositionRetainRuntime,
		})

	var retained *lifecycle.RetainedPromptFailureError
	require.ErrorAs(t, err, &retained)
	session, loadErr := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, loadErr)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	require.Empty(t, mc.sessionMessages)
	task, taskErr := svc.repo.GetTask(ctx, "t1")
	require.NoError(t, taskErr)
	require.Equal(t, v1.TaskStateInProgress, task.State)
}

func TestAgentTurnFailedRoutesAutomationOriginsToTerminalFailureOwner(t *testing.T) {
	for _, origin := range []string{models.TaskOriginAutomationRun, models.TaskOriginAutomationTask} {
		t.Run(origin, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedAutomationTask(t, repo, "t-auto", origin, false)
			now := time.Now().UTC()
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "s-auto", TaskID: "t-auto", State: models.TaskSessionStateRunning,
				StartedAt: now, UpdatedAt: now,
			}))
			seedExecutorRunning(t, repo, "s-auto", "t-auto", "exec-auto")
			session, err := repo.GetTaskSession(ctx, "s-auto")
			require.NoError(t, err)
			session.AgentProfileID = "profile-codex"
			session.AgentProfileSnapshot = map[string]any{"model": "gpt-5-codex"}
			session.DownstreamACPSessionID = "provider-session"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			agentManager := &mockAgentManager{isAgentRunning: true}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
			t.Cleanup(svc.cancelAllTransientRetries)
			svc.executor = agentexecutor.NewExecutor(agentManager, repo, testLogger(), agentexecutor.ExecutorConfig{})
			messageCreator := &mockMessageCreator{}
			svc.messageCreator = messageCreator
			svc.beginPromptAttempt("s-auto", "exec-auto", 9, false)
			svc.observePromptAttempt("s-auto", "exec-auto", 9, true, true)

			var runService interface{}
			if origin == models.TaskOriginAutomationRun {
				automationRuns := &mockAutomationRunService{}
				svc.SetAutomationService(automationRuns)
				runService = automationRuns
			} else {
				require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
					ID: "turn-auto", TaskSessionID: "s-auto", TaskID: "t-auto",
					StartedAt: now, CreatedAt: now, UpdatedAt: now,
				}))
				svc.activeTurns.Store("s-auto", "turn-auto")
				svc.turnService = &repoTurnService{repo: repo}
				binding := &continuityAutomationBindingRecorder{}
				svc.SetAutomationService(binding)
				runService = binding
			}

			data := retainedTurnFailureData()
			data.TaskID = "t-auto"
			data.SessionID = "s-auto"
			data.AgentExecutionID = "exec-auto"
			data.PromptGeneration = 9
			data.TurnID = "turn-auto"
			data.CapacityContinuation = &streams.CapacityContinuationSnapshot{
				Support: streams.CapacityContinuationCodexLiveSessionV1, PromptGeneration: 9,
				EvidenceComplete: true, CompletedTools: 1,
			}
			svc.handleAgentTurnFailed(ctx, data)

			switch automation := runService.(type) {
			case *mockAutomationRunService:
				require.Equal(t, []string{"t-auto"}, automation.failedTaskIDs,
					"automation_run completion must release the task-bound concurrency slot exactly once")
			case *continuityAutomationBindingRecorder:
				require.Equal(t, 1, automation.calls,
					"automation_task completion must settle the exact active run binding once")
				require.Equal(t, "t-auto", automation.taskID)
				require.Equal(t, "s-auto", automation.sessionID)
				require.Equal(t, "turn-auto", automation.turnID)
			default:
				t.Fatalf("unexpected automation failure owner %T", runService)
			}

			waitForStopCall(t, agentManager)
			svc.dynamicSuccessorWorkers.Wait()
			agentManager.mu.Lock()
			require.Len(t, agentManager.stopAgentWithReasonArgs, 1,
				"automation failure must use the existing execution cleanup owner exactly once")
			require.Empty(t, agentManager.capturedPrompts,
				"after-effects automation failures must not dispatch an interactive capacity continuation")
			agentManager.mu.Unlock()
			_, retryOwned := svc.transientRetries.Load("s-auto")
			require.False(t, retryOwned, "automation failure must not create an interactive retained retry")
			for _, message := range messageCreator.sessionMessages {
				require.NotEqual(t, true, message.metadata["runtime_retained"],
					"automation failure must not persist the interactive turn-error presentation")
			}
		})
	}
}

func TestAutomationOwnedCapacityFailureBeforeEffectsKeepsReplayPolicy(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "t-auto", models.TaskOriginAutomationRun, false)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s-auto", TaskID: "t-auto", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))
	seedExecutorRunning(t, repo, "s-auto", "t-auto", "exec-auto")
	agentManager := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.executor = agentexecutor.NewExecutor(agentManager, repo, testLogger(), agentexecutor.ExecutorConfig{})
	svc.SetAutomationService(&mockAutomationRunService{})
	svc.beginPromptAttempt("s-auto", "exec-auto", 9, false)

	data := retainedTurnFailureData()
	data.TaskID = "t-auto"
	data.SessionID = "s-auto"
	data.AgentExecutionID = "exec-auto"
	data.PromptGeneration = 9
	data.OutputObserved = false
	data.EffectObserved = false
	data.CapacityContinuation = nil
	svc.handleAgentTurnFailed(ctx, data)

	value, owned := svc.transientRetries.Load("s-auto")
	require.True(t, owned, "automation's pre-result capacity retry remains eligible")
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.Equal(t, recoveryModeReplay, entry.mode)
	require.Nil(t, entry.continuation)
}

func TestAgentTurnFailedRoutesOfficeTasksToTerminalFailureOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-office", "s-office", "step1")
	task, err := repo.GetTask(ctx, "t-office")
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, repo.UpdateTask(ctx, task))
	agentManager := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	svc.beginPromptAttempt("s-office", "execution-office", 7, false)

	data := retainedTurnFailureData()
	data.TaskID = "t-office"
	data.SessionID = "s-office"
	data.AgentExecutionID = "execution-office"
	svc.handleAgentTurnFailed(ctx, data)

	session, err := repo.GetTaskSession(ctx, "s-office")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, session.State,
		"Office turn failures must use the existing terminal failure owner")
	for _, message := range messageCreator.sessionMessages {
		require.NotEqual(t, true, message.metadata["runtime_retained"],
			"Office failures must not use the interactive retained-turn presentation")
	}
	_, retryOwned := svc.transientRetries.Load("s-office")
	require.False(t, retryOwned, "Office failures must not enter interactive retained retry ownership")
	waitForDynamicSuccessorWorkers(t, svc)
	agentManager.mu.Lock()
	require.Len(t, agentManager.stopAgentWithReasonArgs, 1,
		"the terminal failure owner must clean up the Office execution once")
	agentManager.mu.Unlock()
}

type continuityAutomationBindingRecorder struct {
	mockAutomationRunService
	automationRunBinding
	calls     int
	taskID    string
	sessionID string
	turnID    string
}

func (s *continuityAutomationBindingRecorder) MarkRunTerminalByBinding(
	_ context.Context,
	taskID, sessionID, turnID string,
	status automation.RunStatus,
	errMsg string,
) error {
	s.calls++
	s.taskID, s.sessionID, s.turnID = taskID, sessionID, turnID
	if status != automation.RunStatusFailed {
		return errors.New("automation binding did not receive failed status")
	}
	return nil
}

func TestAgentTurnFailedUsesTerminalRecoveryForDynamicPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageCreator = &mockMessageCreator{}
	svc.beginPromptAttempt("s1", "execution-1", 7, true)

	svc.handleAgentTurnFailed(ctx, retainedTurnFailureData())

	waitForStopCall(t, agentMgr)
	agentMgr.mu.Lock()
	require.Len(t, agentMgr.stopAgentWithReasonArgs, 1)
	agentMgr.mu.Unlock()
}

func retainedTurnFailureData() watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", AgentID: "codex-acp",
		OwnerKind: string(lifecycle.ExecutionOwnerTask), AgentProfileID: "profile-codex", TurnID: "turn-1", PromptGeneration: 7,
		ErrorMessage:             "Selected model is at capacity. Please try a different model.",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		ProviderError: &streams.ProviderError{
			Source: streams.ProviderErrorSourceCodexACP, ProviderID: "codex-acp",
			ErrorKind: "model_capacity", Message: "Selected model is at capacity. Please try a different model.",
		},
		EvidenceKnown: true, OutputObserved: true, EffectObserved: true,
	}
}
