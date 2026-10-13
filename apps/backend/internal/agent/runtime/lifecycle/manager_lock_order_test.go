package lifecycle

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// lockOrderDeadlockGuard only bounds a hung test; the paths finish in
// microseconds once the store reader is released.
const lockOrderDeadlockGuard = 5 * time.Second

// goroutineParkedIn reports whether a goroutine whose stack contains frame is
// blocked on a sync primitive.
func goroutineParkedIn(frames ...string) bool {
	buffer := make([]byte, 4<<20)
	for _, stack := range strings.Split(string(buffer[:runtime.Stack(buffer, true)]), "\n\n") {
		header, _, _ := strings.Cut(stack, "\n")
		matches := strings.Contains(header, "[sync.")
		for _, frame := range frames {
			matches = matches && strings.Contains(stack, frame)
		}
		if matches {
			return true
		}
	}
	return false
}

type lockOrderTransitionResult struct {
	publish func()
	payload *AgentEventPayload
	err     error
}

func newLockOrderTestExecution() *AgentExecution {
	execution := &AgentExecution{
		ID:             "exec-lock-order",
		TaskID:         "task-lock-order",
		SessionID:      "session-lock-order",
		AgentProfileID: "profile-1",
		Status:         v1.AgentStatusRunning,
		WorkspacePath:  "/workspace",
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
	}
	execution.promptGeneration = 1
	execution.setPromptTurnID("turn-lock-order")
	return execution
}

// TestMarkReadyDoesNotDeadlockWithPromptCompletion covers the store-locked
// paths that build an AgentEventPayload. The payload snapshots promptTurnID
// through promptLifecycleMu, while a completion claim holds promptLifecycleMu
// and then waits for the execution store lock. If a payload path takes the
// store lock first, the two block each other forever and every
// GetBySessionID caller blocks behind the store lock.
//
// A held store reader makes each payload path queue for the store lock before
// the completion takes promptLifecycleMu, which is the interleaving observed
// when a cancel escalation raced the agent's late completion.
func TestMarkReadyDoesNotDeadlockWithPromptCompletion(t *testing.T) {
	cases := []struct {
		name    string
		frame   string
		prepare func(*AgentExecution)
		run     func(*Manager, *AgentExecution) lockOrderTransitionResult
	}{
		{
			name:  "cancel escalation marks ready",
			frame: "escalateStuckCancel",
			run: func(m *Manager, execution *AgentExecution) lockOrderTransitionResult {
				return lockOrderTransitionResult{err: m.escalateStuckCancel(context.Background(), execution, nil)}
			},
		},
		{
			name:  "passthrough input marks running",
			frame: "PreparePassthroughRunning",
			prepare: func(execution *AgentExecution) {
				execution.PassthroughProcessID = "pty-lock-order"
				execution.Status = v1.AgentStatusReady
			},
			run: func(m *Manager, execution *AgentExecution) lockOrderTransitionResult {
				publish, err := m.PreparePassthroughRunning(execution.SessionID)
				return lockOrderTransitionResult{publish: publish, err: err}
			},
		},
		{
			name:  "recovered execution becomes available",
			frame: "prepareRecoveredAvailability",
			prepare: func(execution *AgentExecution) {
				execution.recoveredPromptGenerationPending.Store(true)
			},
			run: func(m *Manager, execution *AgentExecution) lockOrderTransitionResult {
				payload, err := m.prepareRecoveredAvailability(context.Background(), execution)
				return lockOrderTransitionResult{payload: payload, err: err}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			execution := newLockOrderTestExecution()
			if tc.prepare != nil {
				tc.prepare(execution)
			}
			require.NoError(t, mgr.executionStore.Add(execution))
			mockBus := mgr.eventBus.(*MockEventBus)
			mockBus.Notify = make(chan struct{}, 4)
			result := assertNoLockOrderDeadlock(t, mgr, execution, tc.frame, tc.run)
			assertLockOrderTransition(t, mgr, execution, mockBus, tc.frame, result)
		})
	}
}

func assertNoLockOrderDeadlock(
	t *testing.T,
	mgr *Manager,
	execution *AgentExecution,
	frame string,
	run func(*Manager, *AgentExecution) lockOrderTransitionResult,
) lockOrderTransitionResult {
	t.Helper()
	mgr.executionStore.mu.RLock()
	readerHeld := true
	releaseReader := func() {
		if readerHeld {
			readerHeld = false
			mgr.executionStore.mu.RUnlock()
		}
	}
	t.Cleanup(releaseReader)

	payloadDone := make(chan lockOrderTransitionResult, 1)
	go func() {
		payloadDone <- run(mgr, execution)
	}()
	require.Eventually(t, func() bool { return goroutineParkedIn(frame, "sync.(*RWMutex).Lock") },
		lockOrderDeadlockGuard, time.Millisecond, "payload path never waited for the store lock")

	completionDone := make(chan bool, 1)
	go func() {
		claim, accepted := mgr.claimPromptCompletion(execution, &agentctl.AgentEvent{
			Type:             "complete",
			PromptGeneration: 1,
			TurnID:           "turn-lock-order",
		}, false)
		if claim.locked {
			execution.promptLifecycleMu.Unlock()
		}
		completionDone <- accepted && claim.locked
	}()
	require.Eventually(t, func() bool { return goroutineParkedIn("claimPromptCompletion") },
		lockOrderDeadlockGuard, time.Millisecond, "completion claim never waited for a lock")

	releaseReader()
	var result lockOrderTransitionResult
	select {
	case result = <-payloadDone:
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatalf("%s deadlocked on promptLifecycleMu and the execution store lock", frame)
	}
	select {
	case accepted := <-completionDone:
		require.True(t, accepted, "completion must claim the original generation")
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatal("prompt completion claim deadlocked")
	}
	_, found := mgr.executionStore.GetBySessionID("another-session")
	require.False(t, found)
	return result
}

func assertLockOrderTransition(t *testing.T, mgr *Manager, execution *AgentExecution,
	mockBus *MockEventBus, frame string, result lockOrderTransitionResult,
) {
	t.Helper()
	require.NoError(t, mgr.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
		require.Same(t, execution, current)
		require.Equal(t, uint64(1), current.promptCompletionGeneration)
		require.Equal(t, v1.AgentStatusReady, current.Status)
	}))
	if frame == "prepareRecoveredAvailability" {
		require.NoError(t, result.err)
		require.NotNil(t, result.payload)
		require.Equal(t, string(v1.AgentStatusReady), result.payload.Status)
		require.Equal(t, "turn-lock-order", result.payload.TurnID)
		require.True(t, result.payload.RecoveredAvailabilityOnly)
		require.False(t, execution.recoveredPromptGenerationPending.Load())
		return
	}
	eventType, status, turnID := events.AgentReady, v1.AgentStatusReady, "turn-lock-order"
	if frame == "PreparePassthroughRunning" {
		require.NoError(t, result.err)
		require.NotNil(t, result.publish)
		result.publish()
		result.publish()
		eventType, status, turnID = events.AgentRunning, v1.AgentStatusRunning, ""
	} else {
		require.ErrorIs(t, result.err, ErrCancelEscalated)
	}
	select {
	case <-mockBus.Notify:
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatal("transition did not publish its event")
	}
	require.Len(t, mockBus.PublishedEvents, 1)
	event := mockBus.PublishedEvents[0]
	require.Equal(t, eventType, event.Type)
	payload := event.Data.(AgentEventPayload)
	require.Equal(t, string(status), payload.Status)
	require.Equal(t, turnID, payload.TurnID)
	require.Equal(t, uint64(1), payload.PromptGeneration)
	require.Equal(t, execution.ID, payload.AgentExecutionID)
}
