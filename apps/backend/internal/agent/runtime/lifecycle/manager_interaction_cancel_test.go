package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctlClient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestManager_CancelAgent_EscalatesWhenAgentHangs reproduces the stuck-turn bug:
// the agent accepts the ACP cancel but never publishes a `complete` event, so the
// in-flight SendPrompt would block forever. The manager must escalate by
// unblocking SendPrompt via promptDoneCh, marking the execution ready, and
// returning ErrCancelEscalated so higher layers can still reconcile DB state.
func TestManager_CancelAgent_EscalatesWhenAgentHangs(t *testing.T) {
	prevWait := cancelWaitTimeout
	prevEsc := cancelEscalationTimeout
	cancelWaitTimeout = 50 * time.Millisecond
	cancelEscalationTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = prevWait
		cancelEscalationTimeout = prevEsc
	})

	// Mock agentctl: ack agent.cancel but never emit a completion event.
	mock := newMockAgentServer(t)
	t.Cleanup(func() { mock.server.Close() })
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.cancel" {
			resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
				"success": true,
			})
			return resp
		}
		return mock.defaultHandler(msg)
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	// Establish the agent stream so the client can send agent.cancel.
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(_ agentctlClient.AgentEvent) {}, nil, nil))
	select {
	case <-mock.wsConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("mock server did not see WS connection")
	}

	mgr := newTestManager(t)
	mockBus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)
	mockBus.Notify = make(chan struct{}, 4)

	promptFinished := make(chan struct{})
	exec := &AgentExecution{
		ID:             "exec-cancel-hang",
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		Status:         v1.AgentStatusRunning,
		WorkspacePath:  "/workspace",
		agentctl:       client,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
		promptFinished: promptFinished,
	}
	mgr.executionStore.Add(exec)

	// Simulate the in-flight SendPrompt: it blocks reading promptDoneCh, and on
	// signal closes promptFinished (the same cleanup beginPromptBarrier's deferred
	// closer does).
	sendPromptDone := make(chan struct{})
	var signal PromptCompletionSignal
	go func() {
		defer close(sendPromptDone)
		signal = <-exec.promptDoneCh
		close(promptFinished)
	}()

	// Tight bounds: escalation window is cancelWaitTimeout + cancelEscalationTimeout
	// (100 ms). Use channel-based synchronization per CLAUDE.md — synctest cannot be
	// used here because the HTTP mock server spawns goroutines outside its bubble.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := mgr.CancelAgent(ctx, exec.ID)
	require.ErrorIs(t, err, ErrCancelEscalated)

	select {
	case <-sendPromptDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("simulated SendPrompt did not release after cancel escalation")
	}
	require.True(t, signal.IsError, "escalation signal must carry IsError=true")
	require.Contains(t, signal.Error, "cancel escalated")

	updated, found := mgr.executionStore.Get(exec.ID)
	require.True(t, found)
	require.Equal(t, v1.AgentStatusReady, updated.Status,
		"execution must be marked ready after cancel escalation so the workflow can proceed")

	// The AgentReady publish is dispatched asynchronously (escalateStuckCancel's
	// asyncPublish=true — see markReadyEventWithContext's doc comment for why).
	// Wait on Notify rather than reading PublishedEvents immediately: the
	// channel receive establishes happens-before with the publishing
	// goroutine's append, so the read below is race-free.
	select {
	case <-mockBus.Notify:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the async AgentReady publish")
	}
	var sawReady bool
	for _, ev := range mockBus.PublishedEvents {
		if ev.Type == events.AgentReady {
			sawReady = true
			break
		}
	}
	require.True(t, sawReady, "expected AgentReady event after cancel escalation")
}

// TestManager_CancelAgent_EscalationCleanupSurvivesCtxCancel covers the case where
// the caller's context is cancelled during the post-escalation wait. Once the
// synthetic signal has been queued on promptDoneCh, the cleanup (MarkReady + drain)
// must still run — otherwise the execution leaks in Running state and the stale
// signal breaks the next PromptAgent call.
func TestManager_CancelAgent_EscalationCleanupSurvivesCtxCancel(t *testing.T) {
	prevWait := cancelWaitTimeout
	prevEsc := cancelEscalationTimeout
	cancelWaitTimeout = 20 * time.Millisecond
	// Long enough that ctx.Done() fires first during the post-escalation wait.
	cancelEscalationTimeout = 500 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = prevWait
		cancelEscalationTimeout = prevEsc
	})

	mock := newMockAgentServer(t)
	t.Cleanup(func() { mock.server.Close() })
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.cancel" {
			resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
				"success": true,
			})
			return resp
		}
		return mock.defaultHandler(msg)
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(_ agentctlClient.AgentEvent) {}, nil, nil))
	select {
	case <-mock.wsConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("mock server did not see WS connection")
	}

	mgr := newTestManager(t)

	// promptFinished is deliberately never closed — simulates a SendPrompt that
	// is blocked on something other than promptDoneCh (so escalation can't
	// release it in time, and our ctx will cancel first).
	exec := &AgentExecution{
		ID:             "exec-cancel-ctx",
		TaskID:         "task-1",
		SessionID:      "session-1",
		Status:         v1.AgentStatusRunning,
		WorkspacePath:  "/workspace",
		agentctl:       client,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
		promptFinished: make(chan struct{}),
	}
	mgr.executionStore.Add(exec)

	// Cancel the caller's context after escalation starts but before the
	// post-escalation wait completes.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := mgr.CancelAgent(ctx, exec.ID)
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"returns ctx error, not ErrCancelEscalated, when caller ctx cancelled")

	// Critical invariant: cleanup ran despite ctx cancellation.
	updated, ok := mgr.executionStore.Get(exec.ID)
	require.True(t, ok)
	require.Equal(t, v1.AgentStatusReady, updated.Status,
		"execution must be marked ready even when caller ctx is cancelled mid-escalation")

	select {
	case sig := <-exec.promptDoneCh:
		t.Fatalf("stale signal must be drained after escalation; got: %+v", sig)
	default:
	}
}

// TestManager_CancelAgent_EscalationDoesNotDeadlockOnReentrantReadySubscriber
// pins the #1653 E2E CI regression: escalateStuckCancel calls MarkReady,
// which publishes events.AgentReady. In production the orchestrator's
// handleAgentReady is registered as a *queue* subscriber for this event, and
// (per the centralized cancelInFlightGuard work on PR #1653) tries to
// re-acquire the very same per-session guard Service.CancelAgent still holds
// at this exact point in the call chain — on the *same goroutine*, since the
// in-memory event bus delivers to queue subscriptions synchronously (see
// bus.MemoryEventBus.publishToQueue's "deliver synchronously to preserve
// ordering" comment). A synchronous inline publish here would have that
// reentrant Lock() call block forever on the non-reentrant mutex, and
// CancelAgent would never return — exactly what the E2E "a message queued
// during a running turn is delivered when the turn is paused" test caught.
//
// This test cannot reproduce the real cross-package reentrancy (it would
// need the real bus.MemoryEventBus plus a real orchestrator.Service), so it
// simulates the hazard directly: OnPublish blocks on a mutex the test holds
// for CancelAgent's entire duration, standing in for the guard a reentrant
// handleAgentReady would try (and fail) to acquire. If escalateStuckCancel
// published inline, CancelAgent itself would call OnPublish and deadlock on
// its own goroutine. Dispatching asynchronously (asyncPublish=true) lets
// CancelAgent return promptly regardless of how long the subscriber blocks.
func TestManager_CancelAgent_EscalationDoesNotDeadlockOnReentrantReadySubscriber(t *testing.T) {
	prevWait := cancelWaitTimeout
	prevEsc := cancelEscalationTimeout
	cancelWaitTimeout = 20 * time.Millisecond
	cancelEscalationTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		cancelWaitTimeout = prevWait
		cancelEscalationTimeout = prevEsc
	})

	mock := newMockAgentServer(t)
	t.Cleanup(func() { mock.server.Close() })
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.cancel" {
			resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
				"success": true,
			})
			return resp
		}
		return mock.defaultHandler(msg)
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(_ agentctlClient.AgentEvent) {}, nil, nil))
	select {
	case <-mock.wsConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("mock server did not see WS connection")
	}

	mgr := newTestManager(t)
	mockBus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)

	// reentrantGuard stands in for the per-session cancelInFlightGuard that
	// the real orchestrator.Service.CancelAgent holds for its own entire
	// call — held here for this whole test, so any synchronous, same-
	// goroutine attempt to acquire it (an inline publish reaching a
	// reentrant handleAgentReady) would block forever.
	var reentrantGuard sync.Mutex
	reentrantGuard.Lock()
	subscriberAttempted := make(chan struct{}, 1)
	subscriberFinished := make(chan struct{})
	mockBus.OnPublish = func(subject string, _ *bus.Event) {
		if subject != events.AgentReady {
			return
		}
		select {
		case subscriberAttempted <- struct{}{}:
		default:
		}
		reentrantGuard.Lock()
		reentrantGuard.Unlock() //nolint:staticcheck // deliberately mirrors a reentrant acquire-then-release
		close(subscriberFinished)
	}

	promptFinished := make(chan struct{})
	exec := &AgentExecution{
		ID:             "exec-cancel-reentrant",
		TaskID:         "task-1",
		SessionID:      "session-1",
		Status:         v1.AgentStatusRunning,
		WorkspacePath:  "/workspace",
		agentctl:       client,
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
		promptFinished: promptFinished,
	}
	require.NoError(t, mgr.executionStore.Add(exec))

	go func() {
		<-exec.promptDoneCh
		close(promptFinished)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cancelDone := make(chan error, 1)
	go func() {
		cancelDone <- mgr.CancelAgent(ctx, exec.ID)
	}()

	select {
	case err := <-cancelDone:
		require.ErrorIs(t, err, ErrCancelEscalated,
			"CancelAgent must return promptly even though its own escalation-triggered AgentReady publish is still blocked on a reentrant acquire")
	case <-time.After(1 * time.Second):
		t.Fatal("CancelAgent deadlocked waiting for its own escalation-triggered ready event to be handled — asyncPublish regression")
	}

	// The simulated subscriber must have at least been reached (proving the
	// event really was published), even though it's still blocked.
	select {
	case <-subscriberAttempted:
	case <-time.After(1 * time.Second):
		t.Fatal("expected the AgentReady publish to reach the simulated subscriber")
	}

	reentrantGuard.Unlock()

	// Wait for the simulated subscriber goroutine to actually finish before
	// returning — otherwise it's still running (blocked, then unblocking)
	// when the test exits, making outcomes scheduler-dependent and risking
	// a goleak false positive.
	select {
	case <-subscriberFinished:
	case <-time.After(1 * time.Second):
		t.Fatal("simulated AgentReady subscriber did not finish after the guard was released")
	}
}

func TestMarkReadyAsync_PublishesImmutablePromptGenerationSnapshot(t *testing.T) {
	mgr := newTestManager(t)
	mockBus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)

	exec := &AgentExecution{
		ID:               "exec-generation-snapshot",
		TaskID:           "task-1",
		SessionID:        "session-1",
		Status:           v1.AgentStatusRunning,
		promptGeneration: 1,
	}
	require.NoError(t, mgr.executionStore.Add(exec))

	publishEntered := make(chan interface{}, 1)
	releasePublish := make(chan struct{})
	mockBus.Notify = make(chan struct{}, 1)
	mockBus.OnPublish = func(subject string, event *bus.Event) {
		if subject != events.AgentReady {
			return
		}
		publishEntered <- event.Data
		<-releasePublish
	}

	require.NoError(t, mgr.markReadyEventWithContext(
		context.Background(), exec.ID, events.AgentReady, true,
	))
	payload, payloadOK := (<-publishEntered).(AgentEventPayload)
	require.True(t, payloadOK)

	_, err := mgr.BeginPrompt(exec.ID)
	require.NoError(t, err)
	require.True(t, mgr.OwnsPromptGeneration(exec.SessionID, exec.ID, 2))
	close(releasePublish)
	<-mockBus.Notify

	require.Equal(t, uint64(1), payload.PromptGeneration)
	require.Equal(t, string(v1.AgentStatusReady), payload.Status)
}
