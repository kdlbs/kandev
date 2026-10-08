package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestPromptAdmissionCaptureRunsBeforeProviderDispatch(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	promptAccepted := make(chan struct{}, 1)
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.prompt" {
			promptAccepted <- struct{}{}
		}
		return mock.defaultHandler(msg)
	}
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(_ agentctl.AgentEvent) {}, nil, nil))
	waitForWSConnected(t, mock)

	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-turn-capture-admission", TaskID: "task-turn-capture-admission",
		SessionID: "session-turn-capture-admission", TaskEnvironmentID: "env-turn-capture-admission",
		Status: v1.AgentStatusRunning, agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.setPromptTurnID("turn-capture-admission")
	require.NoError(t, manager.executionStore.Add(execution))
	handler := &turnChangeCaptureBarrierHandler{
		admitted: make(chan TurnChangeAdmission, 1), releaseAdmission: make(chan struct{}),
	}
	manager.SetTurnChangeCaptureHandler(handler)

	promptDone := make(chan error, 1)
	go func() {
		_, err := manager.PromptAgent(context.Background(), execution.ID, "prompt", nil, true)
		promptDone <- err
	}()
	var admission TurnChangeAdmission
	select {
	case admission = <-handler.admitted:
	case <-time.After(time.Second):
		t.Fatal("admitted-turn capture hook did not run")
	}
	require.Equal(t, "turn-capture-admission", admission.TurnID)
	require.NotZero(t, admission.PromptGeneration)
	select {
	case <-promptAccepted:
		t.Fatal("provider dispatch crossed the blocked start checkpoint")
	default:
	}

	close(handler.releaseAdmission)
	select {
	case <-promptAccepted:
	case <-time.After(time.Second):
		t.Fatal("provider dispatch did not continue after start checkpoint")
	}
	select {
	case err := <-promptDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("prompt did not finish dispatch")
	}
}

func TestLaunchTurnChangeCheckoutsUseTaskEnvironmentManifest(t *testing.T) {
	manager := newTestManager(t)
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-launch-checkouts": {
			TaskID: "task-launch-checkouts", SessionID: "session-launch-checkouts",
			TaskEnvironmentID: "environment-launch-checkouts",
			WorkspaceRepositories: []WorkspaceRepositorySpec{{
				TaskEnvironmentRepoID:  "environment-repo-launch-checkouts",
				TaskRepositoryID:       "task-repo-launch-checkouts",
				RepositoryID:           "repository-launch-checkouts",
				WorktreeID:             "worktree-launch-checkouts",
				RepoName:               "service",
				RepositorySubpath:      "service",
				RepositorySubpathKnown: true,
			}},
		},
	}}
	manager.SetWorkspaceInfoProvider(provider)
	execution := &AgentExecution{
		ID: "execution-launch-checkouts", TaskID: "task-launch-checkouts",
		SessionID: "session-launch-checkouts", TaskEnvironmentID: "environment-launch-checkouts",
	}

	manager.populateLaunchTurnChangeCheckouts(context.Background(), execution)

	require.Equal(t, []TurnChangeCheckout{{
		ID: "environment-repo-launch-checkouts", EnvironmentRepoID: "environment-repo-launch-checkouts",
		TaskRepositoryID: "task-repo-launch-checkouts", RepositoryID: "repository-launch-checkouts",
		WorktreeID: "worktree-launch-checkouts", DisplayName: "service",
		RepositorySubpath: "service", RepositorySubpathKnown: true,
	}}, execution.TurnChangeCheckouts)
	require.Equal(t, 1, provider.sessionCalls)
}

func TestPromptAdmissionRefreshesTurnChangeCheckouts(t *testing.T) {
	manager := newTestManager(t)
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-admission-checkouts": {
			TaskID: "task-admission-checkouts", SessionID: "session-admission-checkouts",
			TaskEnvironmentID: "environment-admission-checkouts",
		},
	}}
	manager.SetWorkspaceInfoProvider(provider)
	execution := &AgentExecution{
		ID: "execution-admission-checkouts", TaskID: "task-admission-checkouts",
		SessionID: "session-admission-checkouts", TaskEnvironmentID: "environment-admission-checkouts",
		promptGeneration: 1, promptTurnIDs: map[uint64]string{1: "turn-admission-checkouts"},
	}
	execution.TurnChangeCheckouts = []TurnChangeCheckout{{ID: "stale-checkout"}}
	require.NoError(t, manager.executionStore.Add(execution))
	handler := &turnChangeCaptureBarrierHandler{admitted: make(chan TurnChangeAdmission, 1)}
	manager.SetTurnChangeCaptureHandler(handler)
	provider.infos[execution.SessionID].WorkspaceRepositories = []WorkspaceRepositorySpec{{
		TaskEnvironmentRepoID: "environment-repo-admission-checkouts",
		TaskRepositoryID:      "task-repo-admission-checkouts", RepositoryID: "repository-admission-checkouts",
		RepositorySubpath: "service", RepositorySubpathKnown: true,
	}}

	require.NoError(t, manager.admitTurnChangeCapture(context.Background(), execution, 1))
	select {
	case admission := <-handler.admitted:
		require.Equal(t, "environment-repo-admission-checkouts", admission.Checkouts[0].ID)
		require.Equal(t, "repository-admission-checkouts", admission.Checkouts[0].RepositoryID)
	case <-time.After(time.Second):
		t.Fatal("turn-change admission was not observed")
	}
	require.Equal(t, 1, provider.sessionCalls)
}

func TestTerminalCaptureFenceBlocksSuccessorAdmission(t *testing.T) {
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-turn-capture-terminal", TaskID: "task-turn-capture-terminal",
		SessionID: "session-turn-capture-terminal", TaskEnvironmentID: "env-turn-capture-terminal",
		Status: v1.AgentStatusRunning, promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.setPromptTurnID("turn-capture-terminal")
	require.NoError(t, manager.executionStore.Add(execution))
	generation, err := manager.BeginPrompt(execution.ID)
	require.NoError(t, err)
	handler := &turnChangeCaptureBarrierHandler{
		terminalEntered: make(chan TurnChangeTerminal, 1), releaseTerminal: make(chan struct{}),
	}
	manager.SetTurnChangeCaptureHandler(handler)
	eventDone := make(chan bool, 1)
	go func() {
		eventDone <- manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: execution.SessionID,
			TurnID: "turn-capture-terminal", PromptGeneration: generation,
		})
	}()
	select {
	case terminal := <-handler.terminalEntered:
		require.Equal(t, generation, terminal.PromptGeneration)
	case <-time.After(time.Second):
		t.Fatal("accepted completion did not enter terminal capture")
	}
	if _, err := manager.BeginPrompt(execution.ID); err != ErrPromptSettlementPending {
		t.Fatalf("successor admission error = %v, want capture-fence rejection", err)
	}
	close(handler.releaseTerminal)
	select {
	case handled := <-eventDone:
		require.True(t, handled)
	case <-time.After(time.Second):
		t.Fatal("terminal handler did not release completion")
	}
	require.Eventually(t, func() bool {
		execution.promptLifecycleMu.Lock()
		defer execution.promptLifecycleMu.Unlock()
		return execution.turnChangeCaptureGeneration == 0
	}, time.Second, 10*time.Millisecond, "successful capture did not release the generation fence")
	if _, err := manager.BeginPrompt(execution.ID); err != nil {
		t.Fatalf("successor remained fenced after terminal capture: %v", err)
	}
}

func TestTerminalCaptureKeepsCompletionResponsiveAndFencesSuccessor(t *testing.T) {
	for _, outcome := range []string{"end_turn", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			manager := newTestManager(t)
			execution := &AgentExecution{
				ID: "execution-terminal-budget-" + outcome, TaskID: "task-terminal-budget",
				SessionID: "session-terminal-budget", TaskEnvironmentID: "env-terminal-budget",
				Status: v1.AgentStatusRunning, promptDoneCh: make(chan PromptCompletionSignal, 1),
			}
			execution.setPromptTurnID("turn-terminal-budget-" + outcome)
			require.NoError(t, manager.executionStore.Add(execution))
			generation, err := manager.BeginPrompt(execution.ID)
			require.NoError(t, err)
			handler := &turnChangeCaptureCompletionBudgetHandler{
				firstDeadline: make(chan time.Time, 1), retryEntered: make(chan struct{}),
				releaseFirst: make(chan struct{}), releaseRetry: make(chan struct{}),
				firstReturned: make(chan struct{}),
			}
			var releaseOnce sync.Once
			var releaseFirstOnce sync.Once
			releaseRetry := func() {
				releaseOnce.Do(func() { close(handler.releaseRetry) })
				releaseFirstOnce.Do(func() { close(handler.releaseFirst) })
			}
			t.Cleanup(releaseRetry)
			manager.SetTurnChangeCaptureHandler(handler)

			started := time.Now()
			eventDone := make(chan bool, 1)
			go func() {
				eventDone <- manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
					Type: "complete", SessionID: execution.SessionID,
					TurnID: "turn-terminal-budget-" + outcome, PromptGeneration: generation,
					Data: map[string]any{"stop_reason": outcome},
				})
			}()
			<-handler.firstDeadline
			var handled bool
			select {
			case handled = <-eventDone:
			case <-time.After(turnChangeTerminalCompletionBudget):
				t.Fatal("terminal capture delayed completion signal")
			}
			completedAt := time.Now()
			require.True(t, handled)
			select {
			case <-handler.firstReturned:
				t.Fatal("terminal capture completed before publishing completion")
			default:
			}
			require.LessOrEqual(t, completedAt.Sub(started), turnChangeTerminalCompletionBudget)
			releaseFirstOnce.Do(func() { close(handler.releaseFirst) })
			select {
			case <-handler.retryEntered:
			case <-time.After(turnChangeTerminalCompletionBudget + time.Second):
				t.Fatal("failed terminal attempt was not retried")
			}
			require.Equal(t, v1.AgentStatusReady, execution.Status)
			if _, err := manager.BeginPrompt(execution.ID); !errors.Is(err, ErrPromptSettlementPending) {
				t.Fatalf("successor admission error = %v, want capture-fence rejection", err)
			}
			releaseRetry()
			require.Eventually(t, func() bool {
				execution.promptLifecycleMu.Lock()
				defer execution.promptLifecycleMu.Unlock()
				return execution.turnChangeCaptureGeneration == 0
			}, 2*time.Second, 10*time.Millisecond, "successful retry did not release the generation fence")
		})
	}
}

func TestStopWaitsForRetriedTerminalCaptureBeforeReturning(t *testing.T) {
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-stop-terminal-retry", TaskID: "task-stop-terminal-retry",
		SessionID: "session-stop-terminal-retry", TaskEnvironmentID: "env-stop-terminal-retry",
		Status: v1.AgentStatusRunning, promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.setPromptTurnID("turn-stop-terminal-retry")
	require.NoError(t, manager.executionStore.Add(execution))
	_, err := manager.BeginPrompt(execution.ID)
	require.NoError(t, err)
	handler := &turnChangeCaptureCompletionBudgetHandler{
		firstDeadline: make(chan time.Time, 1), retryEntered: make(chan struct{}),
		releaseRetry: make(chan struct{}),
	}
	manager.SetTurnChangeCaptureHandler(handler)
	var releaseOnce sync.Once
	releaseRetry := func() { releaseOnce.Do(func() { close(handler.releaseRetry) }) }
	t.Cleanup(releaseRetry)
	stopDone := make(chan struct{})
	go func() {
		manager.preserveTurnChangesBeforeStop(context.Background(), execution)
		close(stopDone)
	}()
	<-handler.firstDeadline
	select {
	case <-handler.retryEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("failed terminal capture was not retried before executor stop")
	}
	select {
	case <-stopDone:
		t.Fatal("executor stop returned while terminal capture retry was pending")
	default:
	}
	releaseRetry()
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("executor stop did not continue after terminal capture settled")
	}
}

func TestStreamDisconnectFinishesAdmittedTurnBeforeErrorPublication(t *testing.T) {
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-turn-capture-disconnect", TaskID: "task-turn-capture-disconnect",
		SessionID: "session-turn-capture-disconnect", TaskEnvironmentID: "env-turn-capture-disconnect",
		Status: v1.AgentStatusRunning, promptGeneration: 1, promptDoneCh: make(chan PromptCompletionSignal, 1),
		promptTurnIDs: map[uint64]string{1: "turn-capture-disconnect"},
	}
	require.NoError(t, manager.executionStore.Add(execution))
	handler := &turnChangeCaptureBarrierHandler{
		terminalEntered: make(chan TurnChangeTerminal, 1), releaseTerminal: make(chan struct{}),
	}
	manager.SetTurnChangeCaptureHandler(handler)
	disconnectDone := make(chan struct{})
	go func() {
		manager.handleStreamDisconnectWithAttempt(execution, errors.New("stream closed"), 1, "")
		close(disconnectDone)
	}()
	select {
	case terminal := <-handler.terminalEntered:
		require.Equal(t, "turn-capture-disconnect", terminal.TurnID)
	case <-time.After(time.Second):
		t.Fatal("stream disconnect did not capture its admitted turn")
	}
	for _, event := range manager.eventBus.(*MockEventBus).PublishedEvents {
		require.NotEqual(t, "agentctl.error", event.Type, "disconnect publication must wait for turn capture")
	}
	close(handler.releaseTerminal)
	select {
	case <-disconnectDone:
	case <-time.After(time.Second):
		t.Fatal("stream disconnect did not finish after terminal capture")
	}
	require.Zero(t, execution.turnChangeCaptureGeneration)
}

func TestStopPreservesActiveTurnBeforeExecutorCleanup(t *testing.T) {
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-turn-capture-stop", TaskID: "task-turn-capture-stop",
		SessionID: "session-turn-capture-stop", TaskEnvironmentID: "env-turn-capture-stop",
		Status: v1.AgentStatusRunning, promptGeneration: 2, promptDoneCh: make(chan PromptCompletionSignal, 1),
		promptTurnIDs: map[uint64]string{2: "turn-capture-stop"},
	}
	require.NoError(t, manager.executionStore.Add(execution))
	handler := &turnChangeCaptureBarrierHandler{
		terminalEntered: make(chan TurnChangeTerminal, 1), releaseTerminal: make(chan struct{}),
	}
	manager.SetTurnChangeCaptureHandler(handler)
	stopDone := make(chan struct{})
	go func() {
		manager.preserveTurnChangesBeforeStop(context.Background(), execution)
		close(stopDone)
	}()
	select {
	case terminal := <-handler.terminalEntered:
		require.Equal(t, "turn-capture-stop", terminal.TurnID)
		require.Equal(t, "stopped", terminal.Outcome)
		require.Empty(t, terminal.FinalAssistantMessageID)
	case <-time.After(time.Second):
		t.Fatal("executor stop did not preserve the active turn")
	}
	close(handler.releaseTerminal)
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("executor stop remained blocked after terminal capture")
	}
	require.Zero(t, execution.turnChangeCaptureGeneration)
}

func TestTerminalCaptureRetainsLastAssistantMessageAcrossFlushAndProtocolReset(t *testing.T) {
	for _, test := range []struct {
		name   string
		before func(*Manager, *AgentExecution, uint64)
	}{
		{
			name: "last of multiple legacy replies",
			before: func(manager *Manager, execution *AgentExecution, generation uint64) {
				manager.publishStreamingMessage(execution, "first", generation, "")
				manager.flushMessageBuffer(execution, generation, "")
				manager.publishStreamingMessage(execution, "final", generation, "")
			},
		},
		{
			name: "protocol-correlated final reply",
			before: func(manager *Manager, execution *AgentExecution, generation uint64) {
				manager.publishProtocolMessage(execution, "protocol-1", "first", generation, "")
				manager.publishProtocolMessage(execution, "protocol-2", "final", generation, "")
			},
		},
		{
			name: "reply created by no-newline completion flush",
			before: func(_ *Manager, execution *AgentExecution, _ uint64) {
				execution.messageMu.Lock()
				execution.messageBuffer.WriteString("final without newline")
				execution.messageMu.Unlock()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := newTestManager(t)
			execution := &AgentExecution{
				ID: "execution-final-anchor-" + test.name, TaskID: "task-final-anchor",
				SessionID: "session-final-anchor", TaskEnvironmentID: "env-final-anchor",
				Status: v1.AgentStatusRunning, promptDoneCh: make(chan PromptCompletionSignal, 1),
			}
			execution.setPromptTurnID("turn-final-anchor")
			require.NoError(t, manager.executionStore.Add(execution))
			generation, err := manager.BeginPrompt(execution.ID)
			require.NoError(t, err)
			test.before(manager, execution, generation)
			execution.messageMu.Lock()
			expectedMessageID := execution.lastAssistantMessageIDByGeneration[generation]
			execution.messageMu.Unlock()
			handler := &turnChangeCaptureBarrierHandler{terminalEntered: make(chan TurnChangeTerminal, 1)}
			manager.SetTurnChangeCaptureHandler(handler)

			require.True(t, manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
				Type: "complete", SessionID: execution.SessionID, TurnID: "turn-final-anchor", PromptGeneration: generation,
			}))
			select {
			case terminal := <-handler.terminalEntered:
				require.NotEmpty(t, terminal.FinalAssistantMessageID)
				if expectedMessageID != "" {
					require.Equal(t, expectedMessageID, terminal.FinalAssistantMessageID)
				}
			case <-time.After(time.Second):
				t.Fatal("terminal capture did not receive the assistant reply anchor")
			}
		})
	}
}

func TestTerminalCaptureRetriesPersistenceAndReleasesFence(t *testing.T) {
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution-terminal-anchor-retry", TaskID: "task-terminal-anchor-retry",
		SessionID: "session-terminal-anchor-retry", TaskEnvironmentID: "env-terminal-anchor-retry",
		Status: v1.AgentStatusRunning, promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.setPromptTurnID("turn-terminal-anchor-retry")
	require.NoError(t, manager.executionStore.Add(execution))
	generation, err := manager.BeginPrompt(execution.ID)
	require.NoError(t, err)
	manager.publishStreamingMessage(execution, "final reply", generation, "")
	execution.messageMu.Lock()
	messageID := execution.lastAssistantMessageIDByGeneration[generation]
	execution.messageMu.Unlock()
	handler := &turnChangeCaptureRetryHandler{terminals: make(chan TurnChangeTerminal, 2)}
	manager.SetTurnChangeCaptureHandler(handler)

	require.True(t, manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
		Type: "complete", SessionID: execution.SessionID, TurnID: "turn-terminal-anchor-retry", PromptGeneration: generation,
	}))
	firstTerminal := <-handler.terminals
	require.Equal(t, messageID, firstTerminal.FinalAssistantMessageID)
	select {
	case retryTerminal := <-handler.terminals:
		require.Equal(t, firstTerminal, retryTerminal)
	case <-time.After(2 * time.Second):
		t.Fatal("terminal persistence was not retried")
	}
	require.Eventually(t, func() bool {
		execution.promptLifecycleMu.Lock()
		defer execution.promptLifecycleMu.Unlock()
		return execution.turnChangeCaptureGeneration == 0
	}, 2*time.Second, 10*time.Millisecond, "successful retry did not release the generation fence")
	execution.messageMu.Lock()
	_, anchorRetained := execution.lastAssistantMessageIDByGeneration[generation]
	execution.messageMu.Unlock()
	require.False(t, anchorRetained)
	execution.promptLifecycleMu.Lock()
	require.Zero(t, execution.turnChangeCaptureGeneration)
	execution.promptLifecycleMu.Unlock()
	nextGeneration, err := manager.BeginPrompt(execution.ID)
	require.NoError(t, err)
	require.Equal(t, generation+1, nextGeneration)
}

type turnChangeCaptureRetryHandler struct {
	mu        sync.Mutex
	attempts  int
	terminals chan TurnChangeTerminal
}

type turnChangeCaptureCompletionBudgetHandler struct {
	mu            sync.Mutex
	attempts      int
	firstDeadline chan time.Time
	releaseFirst  chan struct{}
	firstReturned chan struct{}
	retryEntered  chan struct{}
	releaseRetry  chan struct{}
}

func (h *turnChangeCaptureCompletionBudgetHandler) AdmitTurnChanges(context.Context, TurnChangeAdmission, TurnChangeCheckpointClient) error {
	return nil
}

func (h *turnChangeCaptureCompletionBudgetHandler) FinishTurnChanges(ctx context.Context, _ TurnChangeTerminal, _ TurnChangeCheckpointClient) error {
	h.mu.Lock()
	h.attempts++
	attempt := h.attempts
	h.mu.Unlock()
	if attempt == 1 {
		deadline, _ := ctx.Deadline()
		h.firstDeadline <- deadline
		if h.releaseFirst != nil {
			select {
			case <-ctx.Done():
				close(h.firstReturned)
				return ctx.Err()
			case <-h.releaseFirst:
				close(h.firstReturned)
				return errors.New("transient terminal capture failure")
			}
		}
		timer := time.NewTimer(1500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			if h.firstReturned != nil {
				close(h.firstReturned)
			}
			return ctx.Err()
		case <-timer.C:
			if h.firstReturned != nil {
				close(h.firstReturned)
			}
			return context.DeadlineExceeded
		}
	}
	close(h.retryEntered)
	<-h.releaseRetry
	return nil
}

func (h *turnChangeCaptureRetryHandler) AdmitTurnChanges(context.Context, TurnChangeAdmission, TurnChangeCheckpointClient) error {
	return nil
}

func (h *turnChangeCaptureRetryHandler) FinishTurnChanges(_ context.Context, terminal TurnChangeTerminal, _ TurnChangeCheckpointClient) error {
	h.mu.Lock()
	h.attempts++
	attempt := h.attempts
	h.mu.Unlock()
	h.terminals <- terminal
	if attempt == 1 {
		return errors.New("terminal persistence unavailable")
	}
	return nil
}

var _ TurnChangeCaptureHandler = (*turnChangeCaptureRetryHandler)(nil)

type turnChangeCaptureBarrierHandler struct {
	admitted         chan TurnChangeAdmission
	releaseAdmission chan struct{}
	terminalEntered  chan TurnChangeTerminal
	releaseTerminal  chan struct{}
	finishErr        error
}

func (h *turnChangeCaptureBarrierHandler) AdmitTurnChanges(_ context.Context, admission TurnChangeAdmission, _ TurnChangeCheckpointClient) error {
	if h.admitted != nil {
		h.admitted <- admission
	}
	if h.releaseAdmission != nil {
		<-h.releaseAdmission
	}
	return nil
}

func (h *turnChangeCaptureBarrierHandler) FinishTurnChanges(_ context.Context, terminal TurnChangeTerminal, _ TurnChangeCheckpointClient) error {
	if h.terminalEntered != nil {
		h.terminalEntered <- terminal
	}
	if h.releaseTerminal != nil {
		<-h.releaseTerminal
	}
	return h.finishErr
}

var _ TurnChangeCaptureHandler = (*turnChangeCaptureBarrierHandler)(nil)
