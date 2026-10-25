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
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestManager_ResetAgentContext_ResetRequestTimeout(t *testing.T) {
	mgr := newTestManager(t)
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.suppressSessionResetResponse = true
	resetReceived := make(chan struct{})
	mock.onReset = func() { close(resetReceived) }

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, nil))

	exec := &AgentExecution{
		ID:                 "exec-reset-timeout",
		TaskID:             "task-reset-timeout",
		SessionID:          "session-reset-timeout",
		AgentProfileID:     "profile-reset-timeout",
		ACPSessionID:       "old-session",
		AgentCommand:       "auggie --model test",
		Status:             v1.AgentStatusRunning,
		WorkspacePath:      "/workspace",
		sessionInitialized: true,
		agentctl:           client,
	}
	require.NoError(t, mgr.executionStore.Add(exec))

	requestCtx, requestCancel := context.WithCancel(context.Background())
	t.Cleanup(requestCancel)
	result := make(chan error, 1)
	go func() { result <- mgr.ResetAgentContext(requestCtx, exec.ID) }()

	select {
	case <-resetReceived:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the provider to receive the reset request")
	}

	// A terminal stop is the lifecycle operation used by session deletion. Start
	// it while the reset request is still outstanding so the assertion proves
	// the reset lock is released when the provider deadline expires.
	stopResult := make(chan error, 1)
	go func() {
		stopResult <- mgr.StopAgentWithReason(context.Background(), exec.ID, StopReasonTaskDeleted, true)
	}()
	select {
	case err := <-stopResult:
		t.Fatalf("terminal stop completed before reset cleanup returned: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ResetAgentContext error = %v, want caller deadline", err)
		}
	case <-time.After(11 * time.Second):
		requestCancel()
		err := <-result
		t.Fatalf("ResetAgentContext did not return from its bounded request wait; outer cancellation returned %v", err)
	}

	require.Equal(t, []string{"agent.session.reset"}, mock.getWSActions())
	require.Empty(t, mock.getHTTPActions(), "a cancelled reset request must not enter restart fallback")

	select {
	case err := <-stopResult:
		require.NoError(t, err, "terminal stop must acquire the released reset lock")
	case <-time.After(time.Second):
		t.Fatal("terminal stop remained blocked after the reset request deadline")
	}
	if _, exists := mgr.executionStore.Get(exec.ID); exists {
		t.Fatal("terminal stop did not remove the execution after reset cleanup")
	}
	assertResetDidNotPublishSuccessEvents(t, mgr)
}

func TestManager_ResetAgentContext_CallerDeadlinePrecedesRequestTimeout(t *testing.T) {
	mgr := newTestManager(t)
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.suppressSessionResetResponse = true
	resetReceived := make(chan struct{})
	mock.onReset = func() { close(resetReceived) }

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(agentctl.AgentEvent) {}, nil, nil))

	exec := &AgentExecution{
		ID:                 "exec-reset-caller-deadline",
		TaskID:             "task-reset-caller-deadline",
		SessionID:          "session-reset-caller-deadline",
		AgentProfileID:     "profile-reset-caller-deadline",
		ACPSessionID:       "old-session",
		AgentCommand:       "auggie --model test",
		Status:             v1.AgentStatusRunning,
		WorkspacePath:      "/workspace",
		sessionInitialized: true,
		agentctl:           client,
	}
	require.NoError(t, mgr.executionStore.Add(exec))

	requestCtx, requestCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(requestCancel)
	result := make(chan error, 1)
	started := time.Now()
	go func() { result <- mgr.ResetAgentContext(requestCtx, exec.ID) }()

	select {
	case <-resetReceived:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the provider to receive the reset request")
	}

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(started), time.Second, "caller deadline should beat the internal request timeout")
	case <-time.After(time.Second):
		t.Fatal("ResetAgentContext did not honor the caller deadline")
	}

	require.Equal(t, []string{"agent.session.reset"}, mock.getWSActions())
	require.Empty(t, mock.getHTTPActions(), "a cancelled reset request must not enter restart fallback")
	assertResetDidNotPublishSuccessEvents(t, mgr)
}

func TestManager_ResetAgentContext_DelayedProviderSessionIsFenced(t *testing.T) {
	mgr := newTestManager(t)
	mock := newRestartMockAgentctlServer(t, false, false)
	mock.resetResponseDelay = 250 * time.Millisecond
	mock.resetLateEventSent = make(chan struct{})
	mock.resetLateEvent = &agentctl.AgentEvent{
		Type:           streams.EventTypeSessionModels,
		SessionID:      "late-session",
		CurrentModelID: "late-model",
		SessionModels:  []streams.SessionModelInfo{{ModelID: "late-model"}},
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	lateEventReceived := make(chan struct{})
	receivedOnce := sync.Once{}
	exec := &AgentExecution{
		ID:                 "exec-reset-delayed",
		TaskID:             "task-reset-delayed",
		SessionID:          "session-reset-delayed",
		AgentProfileID:     "profile-reset-delayed",
		ACPSessionID:       "old-session",
		AgentCommand:       "auggie --model test",
		Status:             v1.AgentStatusRunning,
		WorkspacePath:      "/workspace",
		sessionInitialized: true,
		agentctl:           client,
	}
	exec.SetModelState(&CachedModelState{
		CurrentModelID: "old-model",
		Models:         []streams.SessionModelInfo{{ModelID: "old-model"}},
	})
	require.NoError(t, mgr.executionStore.Add(exec))
	require.NoError(t, client.StreamUpdates(streamCtx, func(event agentctl.AgentEvent) {
		receivedOnce.Do(func() { close(lateEventReceived) })
		mgr.handleAgentEvent(exec, event)
	}, nil, nil))

	requestCtx, requestCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer requestCancel()
	result := make(chan error, 1)
	go func() { result <- mgr.ResetAgentContext(requestCtx, exec.ID) }()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("ResetAgentContext did not honor the caller deadline")
	}
	require.ErrorIs(t, exec.contextResetAdmissionError(), ErrContextResetFenced)

	select {
	case <-mock.resetLateEventSent:
	case <-time.After(time.Second):
		t.Fatal("mock provider did not emit the delayed replacement-session event")
	}
	select {
	case <-lateEventReceived:
	case <-time.After(time.Second):
		t.Fatal("lifecycle stream did not receive the delayed replacement-session event")
	}

	state := exec.GetModelState()
	require.Nil(t, state, "late setup events must not repopulate the discarded reset catalog")
	require.Equal(t, "old-session", exec.ACPSessionID)
	_, err := mgr.PromptAgent(context.Background(), exec.ID, "late prompt", nil, true)
	require.ErrorIs(t, err, ErrContextResetFenced)
}

func assertResetDidNotPublishSuccessEvents(t *testing.T, mgr *Manager) {
	t.Helper()
	managerEvents := mgr.eventBus.(*MockEventBus).PublishedEvents
	for _, event := range managerEvents {
		require.NotEqual(t, events.AgentContextReset, event.Type)
		require.NotEqual(t, events.AgentBootReady, event.Type)
	}
}
