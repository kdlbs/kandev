package lifecycle

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
func TestLateCompletionCoalescerAndSettlementAcknowledgementProgress(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	manager, execution := fixture.manager, fixture.execution
	execution.Owner = ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: execution.TaskID, SessionID: execution.SessionID}
	execution.TaskScope = TaskLaunchScopeTask
	execution.AgentProfileID = "profile-codex"
	execution.AgentID = "codex-acp"
	execution.setSessionInitialized(true)
	eventBus := bus.NewMemoryEventBus(newTestLogger())
	t.Cleanup(eventBus.Close)
	manager.eventBus = eventBus
	manager.eventPublisher = NewEventPublisher(eventBus, manager.logger)

	var sessionGuard sync.Mutex
	var published []string
	publicationEntered := make(chan struct{})
	_, err := eventBus.Subscribe(events.BuildAgentStreamSubject(execution.SessionID), func(_ context.Context, event *bus.Event) error {
		payload := event.Data.(AgentStreamEventPayload)
		if payload.Data.MessageID != "lock-order-message" {
			return nil
		}
		if payload.Data.Text == " tail" {
			close(publicationEntered)
		}
		sessionGuard.Lock()
		defer sessionGuard.Unlock()
		published = append(published, payload.Data.Text)
		return nil
	})
	require.NoError(t, err)

	coalescer := manager.streamCoalescer(execution)
	coalescer.window = time.Hour
	coalescer.add(coalescedStreamChunk{
		eventType: "message_streaming", messageID: "lock-order-message", content: "first", promptGeneration: 1,
	})
	coalescer.add(coalescedStreamChunk{
		eventType: "message_streaming", messageID: "lock-order-message", content: " tail", isAppend: true, promptGeneration: 1,
	})
	sessionGuard.Lock()
	var releaseOnce sync.Once
	releaseGuard := func() { releaseOnce.Do(sessionGuard.Unlock) }
	t.Cleanup(releaseGuard)

	publicationDone := make(chan struct{})
	go func() {
		defer close(publicationDone)
		coalescer.flush()
	}()
	t.Cleanup(func() {
		releaseGuard()
		require.Eventually(t, func() bool {
			select {
			case <-publicationDone:
				return true
			default:
				return false
			}
		}, lockOrderDeadlockGuard, time.Millisecond)
	})
	select {
	case <-publicationEntered:
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatal("ordered publisher did not enter the guarded stream consumer")
	}

	completionDone := make(chan struct{})
	var accepted atomic.Bool
	go func() {
		defer close(completionDone)
		accepted.Store(manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: execution.SessionID, PromptGeneration: 1,
			Error: "Selected model is at capacity.", PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
			Data: map[string]any{"is_error": true},
		}))
	}()
	t.Cleanup(func() {
		releaseGuard()
		select {
		case <-completionDone:
		case <-time.After(lockOrderDeadlockGuard):
			t.Error("late completion did not drain during cleanup")
		}
	})
	require.Eventually(t, func() bool {
		return goroutineParkedIn("(*streamCoalescer).flushBoundary", "sync.(*Mutex).Lock")
	}, lockOrderDeadlockGuard, time.Millisecond,
		"late completion must own its prompt claim while waiting for the publisher")

	// The orchestrator retains its operation but releases this physical guard
	// before acknowledgement can wait for the prompt completion claim.
	releaseGuard()
	ackDone := make(chan bool, 1)
	go func() { ackDone <- manager.AcknowledgeRetainedPromptFailure(execution.ID, 1) }()
	var acknowledged bool
	select {
	case acknowledged = <-ackDone:
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatal("settlement acknowledgement did not finish after the session guard was released")
	}
	require.True(t, acknowledged, "the retained turn failure must acknowledge its own settlement")
	select {
	case <-completionDone:
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatal("late completion did not finish after ordered publication")
	}
	require.True(t, accepted.Load())
	sessionGuard.Lock()
	observed := append([]string(nil), published...)
	sessionGuard.Unlock()
	require.Equal(t, []string{"first", " tail"}, observed)
	next, err := manager.BeginPrompt(execution.ID)
	require.NoError(t, err)
	require.Equal(t, uint64(2), next)
	require.False(t, manager.AcknowledgeRetainedPromptFailure(execution.ID, 1),
		"a stale acknowledgement cannot release the successor generation")
}
