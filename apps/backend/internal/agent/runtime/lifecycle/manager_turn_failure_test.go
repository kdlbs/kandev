package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestTransientTurnFailureKeepsExecutionReady(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	execution := fixture.execution
	execution.TaskScope = TaskLaunchScopeTask
	execution.AgentProfileID = "profile-codex"
	execution.AgentID = "mock-agent"
	execution.setSessionInitialized(true)

	accepted := fixture.manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
		Type:                     "complete",
		SessionID:                execution.SessionID,
		PromptGeneration:         1,
		Error:                    "Selected model is at capacity.",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		Data:                     map[string]any{"is_error": true},
	})
	require.True(t, accepted)
	require.Equal(t, v1.AgentStatusReady, execution.Status)
	require.Nil(t, execution.ExitCode)
	require.Nil(t, execution.FinishedAt)

	var turnFailed, failed, ready bool
	for _, event := range fixture.manager.eventBus.(*MockEventBus).PublishedEvents {
		switch event.Type {
		case "agent.turn_failed":
			turnFailed = true
		case "agent.failed":
			failed = true
		case "agent.ready":
			ready = true
		}
	}
	require.True(t, turnFailed, "retained error must publish its distinct turn-failure event")
	require.False(t, failed, "retained turn failure must not publish terminal execution failure")
	require.False(t, ready, "a failed turn must not publish successful readiness")

	select {
	case signal := <-execution.promptDoneCh:
		require.True(t, signal.IsError)
		require.Equal(t, streams.PromptFailureDispositionRetainRuntime, signal.PromptFailureDisposition)
		execution.promptDoneCh <- signal
		_, err := fixture.manager.sessionManager.waitForPromptDone(
			t.Context(), execution, signal.PromptGeneration,
		)
		var retainedFailure *RetainedPromptFailureError
		require.ErrorAs(t, err, &retainedFailure)
		require.ErrorIs(t, err, ErrAgentReported)
	case <-time.After(time.Second):
		t.Fatal("failed turn did not release its blocking prompt caller")
	}
}

func TestTransientTurnFailurePublishesOutsidePromptLockAndFencesSuccessor(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	execution := fixture.execution
	execution.Owner = ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: execution.TaskID, SessionID: execution.SessionID}
	execution.TaskScope = TaskLaunchScopeTask
	execution.AgentProfileID = "profile-codex"
	execution.AgentID = "codex-acp"
	execution.setSessionInitialized(true)

	eventBus := bus.NewMemoryEventBus(newTestLogger())
	fixture.manager.eventBus = eventBus
	fixture.manager.eventPublisher = NewEventPublisher(eventBus, fixture.manager.logger)
	var cancelGuard sync.Mutex
	callbackEntered := make(chan struct{})
	callbackResult := make(chan struct {
		generation uint64
		next       uint64
		nextErr    error
		err        error
	}, 1)
	_, err := eventBus.Subscribe(events.AgentTurnFailed, func(ctx context.Context, _ *bus.Event) error {
		close(callbackEntered)
		cancelGuard.Lock()
		defer cancelGuard.Unlock()
		generation, generationErr := fixture.manager.GetPromptGenerationForSession(ctx, execution.SessionID)
		next, nextErr := fixture.manager.BeginPrompt(execution.ID)
		callbackResult <- struct {
			generation uint64
			next       uint64
			nextErr    error
			err        error
		}{generation: generation, next: next, nextErr: nextErr, err: generationErr}
		return nil
	})
	require.NoError(t, err)

	guardHeld := make(chan struct{})
	allowCapture := make(chan struct{})
	cancelResult := make(chan struct {
		generation uint64
		err        error
	}, 1)
	go func() {
		cancelGuard.Lock()
		close(guardHeld)
		<-allowCapture
		generation, captureErr := fixture.manager.GetPromptGenerationForSession(context.Background(), execution.SessionID)
		cancelGuard.Unlock()
		cancelResult <- struct {
			generation uint64
			err        error
		}{generation: generation, err: captureErr}
	}()
	<-guardHeld

	completionDone := make(chan bool, 1)
	go func() {
		completionDone <- fixture.manager.handleCompleteEvent(execution, retainedCapacityEvent(execution))
	}()
	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("synchronous retained-failure callback did not start")
	}
	close(allowCapture)

	select {
	case captured := <-cancelResult:
		require.NoError(t, captured.err)
		require.Equal(t, uint64(1), captured.generation)
	case <-time.After(time.Second):
		t.Fatal("explicit-cancel generation capture deadlocked with retained completion")
	}
	select {
	case result := <-callbackResult:
		require.NoError(t, result.err)
		require.Equal(t, uint64(1), result.generation)
		require.ErrorIs(t, result.nextErr, ErrPromptSettlementPending,
			"a synchronous subscriber must not admit a successor before settlement completes")
		require.Zero(t, result.next, "the failed generation must remain owner during publication")
	case <-time.After(time.Second):
		t.Fatal("retained-failure callback did not finish")
	}
	select {
	case accepted := <-completionDone:
		require.True(t, accepted)
	case <-time.After(time.Second):
		t.Fatal("retained completion did not finish after its synchronous callback")
	}

	execution.promptLifecycleMu.Lock()
	settlementGeneration := execution.promptSettlementGeneration
	execution.promptLifecycleMu.Unlock()
	require.Equal(t, uint64(1), settlementGeneration,
		"publication completion is not proof that durable turn settlement finished")
	_, err = fixture.manager.BeginPrompt(execution.ID)
	require.ErrorIs(t, err, ErrPromptSettlementPending,
		"a prompt must remain fenced until its durable owner acknowledges the generation")

	acknowledger, ok := any(fixture.manager).(interface {
		AcknowledgeRetainedPromptFailure(string, uint64) bool
	})
	require.True(t, ok, "lifecycle must expose exact-generation settlement acknowledgement")
	require.True(t, acknowledger.AcknowledgeRetainedPromptFailure(execution.ID, 1),
		"the durable owner may release the exact settled generation")
	successor, err := fixture.manager.BeginPrompt(execution.ID)
	require.NoError(t, err)
	require.Equal(t, uint64(2), successor, "the successor may be admitted after the old settlement signal")
	require.False(t, acknowledger.AcknowledgeRetainedPromptFailure(execution.ID, 1),
		"a stale acknowledgement must not release or alter a successor generation")
	require.Equal(t, successor, execution.promptGenerationSnapshot())
}

func TestTransientTurnFailurePublicationFailureUsesTerminalFallback(t *testing.T) {
	fixture := readyForRetainedTurnFailure(t)
	failingBus := &rejectAgentTurnFailedBus{
		MockEventBus: fixture.manager.eventBus.(*MockEventBus),
	}
	fixture.manager.eventBus = failingBus
	fixture.manager.eventPublisher = NewEventPublisher(failingBus, fixture.manager.logger)

	require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, retainedCapacityEvent(fixture.execution)))
	require.Equal(t, v1.AgentStatusFailed, fixture.execution.Status,
		"a failed retained-event publish must use the ordinary terminal failure path")
	require.NotNil(t, fixture.execution.ExitCode)
	require.NotNil(t, fixture.execution.FinishedAt)

	var turnFailed, failed bool
	for _, published := range failingBus.PublishedEvents {
		switch published.Type {
		case events.AgentTurnFailed:
			turnFailed = true
		case events.AgentFailed:
			failed = true
		}
	}
	require.False(t, turnFailed, "the bus must reject the retained-turn publication")
	require.True(t, failed, "terminal failure must be published after the retained event is rejected")
}

type rejectAgentTurnFailedBus struct {
	*MockEventBus
}

func (b *rejectAgentTurnFailedBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if subject == events.AgentTurnFailed {
		return errors.New("simulated NATS publish failure")
	}
	return b.MockEventBus.Publish(ctx, subject, event)
}

func TestRetainedPromptFailureEligibilityUsesCurrentAgentCtlClient(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, nil))
	waitForWSConnected(t, mock)

	execution := &AgentExecution{
		ID: "exec-current-client", TaskID: "task-current-client", SessionID: "session-current-client",
		Status: v1.AgentStatusRunning, AgentProfileID: "profile-codex", AgentID: "codex-acp",
		TaskScope: TaskLaunchScopeTask,
		agentctl:  agentctl.NewClient("127.0.0.1", 1, newTestLogger()),
		Owner:     ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: "task-current-client", SessionID: "session-current-client"},
	}
	execution.setSessionInitialized(true)
	execution.agentctlOverride.Store(client)

	require.True(t, retainedPromptFailureExecutionEligible(execution),
		"retention must inspect the leased override client that owns the live stream")
}

func TestTransientTurnFailureRequiresCurrentEligibleRuntime(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*AgentExecution)
	}{
		{name: "uninitialized session", change: func(e *AgentExecution) { e.setSessionInitialized(false) }},
		{name: "no concrete profile", change: func(e *AgentExecution) { e.AgentProfileID = "" }},
		{name: "run owner", change: func(e *AgentExecution) { e.Owner.Kind = ExecutionOwnerRun }},
		{name: "office scope", change: func(e *AgentExecution) { e.TaskScope = TaskLaunchScopeOffice }},
		{name: "automation scope", change: func(e *AgentExecution) { e.TaskScope = TaskLaunchScopeAutomation }},
		{name: "unknown scope", change: func(e *AgentExecution) { e.TaskScope = TaskLaunchScopeUnknown }},
		{name: "passthrough", change: func(e *AgentExecution) { e.IsPassthrough = true }},
		{name: "unsupported provider", change: func(e *AgentExecution) { e.AgentID = "opencode-acp" }},
		{name: "utility execution", change: func(e *AgentExecution) { e.TaskID = "" }},
		{name: "closed stream", change: func(e *AgentExecution) {
			e.agentctlLifecycleMu.Lock()
			e.agentctl = nil
			e.agentctlLifecycleMu.Unlock()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDispatchCompletionFixture(t, false)
			execution := fixture.execution
			execution.Owner = ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: execution.TaskID, SessionID: execution.SessionID}
			execution.TaskScope = TaskLaunchScopeTask
			execution.AgentProfileID = "profile-codex"
			execution.AgentID = "codex-acp"
			execution.setSessionInitialized(true)
			test.change(execution)

			require.True(t, fixture.manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
				Type: "complete", SessionID: execution.SessionID, PromptGeneration: 1,
				Error: "provider failed", PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
				Data: map[string]any{"is_error": true},
			}))
			require.Equal(t, v1.AgentStatusFailed, execution.Status)
			require.NotNil(t, execution.ExitCode)
			for _, event := range fixture.manager.eventBus.(*MockEventBus).PublishedEvents {
				require.NotEqual(t, "agent.turn_failed", event.Type)
			}
		})
	}
}

func TestTransientTurnFailureDuplicateDoesNotRepublish(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	execution := fixture.execution
	execution.TaskScope = TaskLaunchScopeTask
	execution.AgentProfileID = "profile-codex"
	execution.AgentID = "codex-acp"
	execution.setSessionInitialized(true)
	event := &agentctl.AgentEvent{
		Type: "complete", SessionID: execution.SessionID, PromptGeneration: 1,
		Error: "provider failed", PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		Data: map[string]any{"is_error": true},
	}
	require.True(t, fixture.manager.handleCompleteEvent(execution, event))
	require.False(t, fixture.manager.handleCompleteEvent(execution, event))

	var retained int
	for _, published := range fixture.manager.eventBus.(*MockEventBus).PublishedEvents {
		if published.Type == "agent.turn_failed" {
			retained++
		}
	}
	require.Equal(t, 1, retained)
}

func TestTransientTurnFailureDoesNotOverrideRuntimeDisconnect(t *testing.T) {
	t.Run("disconnect before error event", func(t *testing.T) {
		fixture := readyForRetainedTurnFailure(t)
		execution := fixture.execution
		fixture.manager.handleStreamDisconnectWithAttempt(execution, errors.New("stream closed"), 1, "")

		require.True(t, fixture.manager.handleCompleteEvent(execution, retainedCapacityEvent(execution)))
		require.Equal(t, v1.AgentStatusFailed, execution.Status)
		for _, published := range fixture.manager.eventBus.(*MockEventBus).PublishedEvents {
			require.NotEqual(t, "agent.turn_failed", published.Type)
		}
	})

	t.Run("disconnect after retained turn", func(t *testing.T) {
		fixture := readyForRetainedTurnFailure(t)
		execution := fixture.execution
		require.True(t, fixture.manager.handleCompleteEvent(execution, retainedCapacityEvent(execution)))

		fixture.manager.handleStreamDisconnectWithAttempt(execution, errors.New("stream closed"), 1, "")
		require.Equal(t, v1.AgentStatusFailed, execution.Status)
	})
}

func readyForRetainedTurnFailure(t *testing.T) *dispatchCompletionFixture {
	t.Helper()
	fixture := newDispatchCompletionFixture(t, false)
	execution := fixture.execution
	execution.TaskScope = TaskLaunchScopeTask
	execution.AgentProfileID = "profile-codex"
	execution.AgentID = "codex-acp"
	execution.setSessionInitialized(true)
	return fixture
}

func retainedCapacityEvent(execution *AgentExecution) *agentctl.AgentEvent {
	return &agentctl.AgentEvent{
		Type: "complete", SessionID: execution.SessionID, PromptGeneration: 1,
		Error:                    "Selected model is at capacity. Please try a different model.",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		Data:                     map[string]any{"is_error": true},
	}
}

func TestCursorResourceTurnRetention(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	execution := fixture.execution
	execution.TaskScope = TaskLaunchScopeTask
	execution.AgentProfileID = "profile-cursor"
	execution.AgentID = "cursor-acp"
	execution.setSessionInitialized(true)

	accepted := fixture.manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
		Type:                     "complete",
		SessionID:                execution.SessionID,
		PromptGeneration:         1,
		Error:                    "Error: RetriableError: [resource_exhausted] Error",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		Data:                     map[string]any{"is_error": true},
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceCursorACP,
			ProviderID: "cursor-acp",
			Message:    "Error: RetriableError: [resource_exhausted] Error",
		},
	})
	require.True(t, accepted)
	require.Equal(t, v1.AgentStatusReady, execution.Status)
	require.Nil(t, execution.ExitCode)
	require.Nil(t, execution.FinishedAt)

	var turnFailed, failed, ready bool
	for _, event := range fixture.manager.eventBus.(*MockEventBus).PublishedEvents {
		switch event.Type {
		case "agent.turn_failed":
			turnFailed = true
		case "agent.failed":
			failed = true
		case "agent.ready":
			ready = true
		}
	}
	require.True(t, turnFailed, "retained resource error must publish its distinct turn-failure event")
	require.False(t, failed, "retained turn failure must not publish terminal execution failure")
	require.False(t, ready, "a failed turn must not publish successful readiness")
}
