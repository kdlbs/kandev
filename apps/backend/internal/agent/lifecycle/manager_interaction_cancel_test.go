package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctlClient "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/events"
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

	mgr := newTestManager()

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

	mockBus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)
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

	mgr := newTestManager()

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
