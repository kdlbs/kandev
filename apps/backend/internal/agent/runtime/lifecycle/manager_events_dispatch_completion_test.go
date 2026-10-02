package lifecycle

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type dispatchCompletionFixture struct {
	manager            *Manager
	execution          *AgentExecution
	mock               *mockAgentServer
	promptGenerations  chan uint64
	completionAccepted chan bool
}

func newDispatchCompletionFixture(t *testing.T, completeBeforeAcknowledgement bool) *dispatchCompletionFixture {
	t.Helper()
	var onPrompt func(*dispatchCompletionFixture, uint64)
	if completeBeforeAcknowledgement {
		onPrompt = func(fixture *dispatchCompletionFixture, generation uint64) {
			if generation != 1 {
				return
			}
			fixture.completionAccepted <- fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
				Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: generation,
			})
		}
	}
	fixture := setupDispatchCompletionFixture(t, nil, onPrompt)
	_, err := fixture.manager.PromptAgent(context.Background(), fixture.execution.ID, "dispatch-only", nil, true)
	require.NoError(t, err)
	require.Equal(t, uint64(1), receiveDispatchPromptGeneration(t, fixture.promptGenerations))
	if completeBeforeAcknowledgement {
		require.True(t, receiveDispatchCompletion(t, fixture.completionAccepted),
			"the completion delivered during agent.prompt must claim the admitted generation")
	}
	return fixture
}

func setupDispatchCompletionFixture(
	t *testing.T,
	prepare func(*dispatchCompletionFixture),
	onPrompt func(*dispatchCompletionFixture, uint64),
) *dispatchCompletionFixture {
	t.Helper()

	manager := newTestManager(t)
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	execution := &AgentExecution{
		ID:            "exec-dispatch-completion",
		TaskID:        "task-dispatch-completion",
		SessionID:     "session-dispatch-completion",
		Status:        v1.AgentStatusRunning,
		WorkspacePath: "/workspace",
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	promptGenerations := make(chan uint64, 4)
	completionAccepted := make(chan bool, 1)
	fixture := &dispatchCompletionFixture{
		manager:            manager,
		execution:          execution,
		mock:               mock,
		promptGenerations:  promptGenerations,
		completionAccepted: completionAccepted,
	}
	if prepare != nil {
		prepare(fixture)
	}
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.prompt" {
			prompt, exists := manager.executionStore.promptLifecycleSnapshot(execution.ID)
			if exists {
				promptGenerations <- prompt.generation
				if onPrompt != nil {
					onPrompt(fixture, prompt.generation)
				}
			}
		}
		return mock.defaultHandler(msg)
	}

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(_ agentctl.AgentEvent) {}, nil, nil))
	waitForWSConnected(t, mock)

	execution.agentctl = client
	require.NoError(t, manager.executionStore.Add(execution))
	return fixture
}

func receiveDispatchPromptGeneration(t *testing.T, generations <-chan uint64) uint64 {
	t.Helper()
	select {
	case generation := <-generations:
		return generation
	case <-time.After(time.Second):
		t.Fatal("agent.prompt did not reach the mock agent")
		return 0
	}
}

func receiveDispatchCompletion(t *testing.T, completions <-chan bool) bool {
	t.Helper()
	select {
	case accepted := <-completions:
		return accepted
	case <-time.After(time.Second):
		t.Fatal("completion did not arrive before the prompt acknowledgement")
		return false
	}
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.1, AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.3
func TestDispatchCompletion_WakeupAfterNumberedCompletion(t *testing.T) {
	for _, order := range []struct {
		name                          string
		completeBeforeAcknowledgement bool
	}{
		{name: "acknowledgement_then_completion"},
		{name: "completion_then_acknowledgement", completeBeforeAcknowledgement: true},
	} {
		t.Run(order.name, func(t *testing.T) {
			fixture := newDispatchCompletionFixture(t, order.completeBeforeAcknowledgement)
			if !order.completeBeforeAcknowledgement {
				require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
					Type:             "complete",
					SessionID:        fixture.execution.SessionID,
					PromptGeneration: 1,
				}))
			}

			require.False(t, fixture.execution.dispatchedPromptPending.Load(),
				"a completed numbered prompt must release its dispatch barrier")
			require.Equal(t, v1.AgentStatusReady, fixture.execution.Status)

			fixture.manager.handleAgentEvent(fixture.execution, agentctl.AgentEvent{
				Type: "message_chunk",
				Text: "autonomous follow-up",
			})
			require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
				Type:      "complete",
				SessionID: fixture.execution.SessionID,
			}), "the autonomous completion must be accepted after its predecessor settles")
			require.Equal(t, v1.AgentStatusReady, fixture.execution.Status)
		})
	}
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.3, AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.4
func TestDispatchCompletion_AcknowledgementOwnership(t *testing.T) {
	t.Run("completion_wins", func(t *testing.T) {
		manager := newTestManager(t)
		execution := createTestExecution("exec-ack-completed", "task-ack", "session-ack")
		require.NoError(t, manager.executionStore.Add(execution))
		generation, err := manager.executionStore.BeginPrompt(execution.ID)
		require.NoError(t, err)
		manager.executionStore.MarkPromptDispatched(execution.ID, generation)
		require.True(t, manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: execution.SessionID, PromptGeneration: generation,
		}))

		callbackCalled := false
		callbackUnlocked := false
		_, err = manager.sessionManager.finishAcceptedPrompt(
			context.Background(), execution, true, func() {
				callbackCalled = true
				callbackUnlocked = execution.promptLifecycleMu.TryLock()
				if callbackUnlocked {
					execution.promptLifecycleMu.Unlock()
				}
			}, generation,
		)
		require.NoError(t, err)
		require.True(t, callbackCalled, "dispatch callback must still be delivered")
		require.True(t, callbackUnlocked, "dispatch callback must run after lifecycle locks are released")
		require.False(t, execution.dispatchedPromptPending.Load(),
			"a late acknowledgement must not restore a completed prompt barrier")
	})

	t.Run("successor_owns_state", func(t *testing.T) {
		manager := newTestManager(t)
		execution := createTestExecution("exec-ack-successor", "task-ack", "session-ack")
		require.NoError(t, manager.executionStore.Add(execution))
		predecessor, err := manager.executionStore.BeginPrompt(execution.ID)
		require.NoError(t, err)
		manager.executionStore.MarkPromptDispatched(execution.ID, predecessor)
		successor, err := manager.executionStore.BeginPrompt(execution.ID)
		require.NoError(t, err)
		require.Greater(t, successor, predecessor)

		_, err = manager.sessionManager.finishAcceptedPrompt(
			context.Background(), execution, true, nil, predecessor,
		)
		require.NoError(t, err)
		require.False(t, execution.dispatchedPromptPending.Load(),
			"a predecessor acknowledgement must not create a successor's barrier")
	})

	t.Run("replacement_execution_owns_state", func(t *testing.T) {
		manager := newTestManager(t)
		oldExecution := createTestExecution("exec-ack-replacement", "task-ack", "session-ack")
		require.NoError(t, manager.executionStore.Add(oldExecution))
		generation, err := manager.executionStore.BeginPrompt(oldExecution.ID)
		require.NoError(t, err)
		manager.executionStore.MarkPromptDispatched(oldExecution.ID, generation)
		manager.executionStore.Remove(oldExecution.ID)

		replacement := createTestExecution(oldExecution.ID, oldExecution.TaskID, oldExecution.SessionID)
		require.NoError(t, manager.executionStore.Add(replacement))
		_, err = manager.sessionManager.finishAcceptedPrompt(
			context.Background(), oldExecution, true, nil, generation,
		)
		require.NoError(t, err)
		require.False(t, oldExecution.dispatchedPromptPending.Load(),
			"a removed execution must not retain acknowledgement state")
		require.False(t, replacement.dispatchedPromptPending.Load(),
			"an acknowledgement for a removed execution must not mutate its replacement")
	})

	t.Run("standalone_session_manager", func(t *testing.T) {
		execution := createTestExecution("exec-ack-standalone", "task-ack", "session-ack")
		execution.promptGeneration = 3
		execution.dispatchedPromptGeneration = 3
		manager := NewSessionManager(newTestLogger(), make(chan struct{}))
		_, err := manager.finishAcceptedPrompt(context.Background(), execution, true, nil, 3)
		require.NoError(t, err)
		require.True(t, execution.dispatchedPromptPending.Load(),
			"the no-store path must retain a live dispatched prompt barrier")
	})
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.4
func TestDispatchCompletion_StaleAndDuplicateEvents(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	first := &agentctl.AgentEvent{
		Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
	}
	require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, first))
	require.False(t, fixture.manager.handleCompleteEvent(fixture.execution, first),
		"a duplicate completion must not claim or flush the finished generation")

	_, err := fixture.manager.PromptAgent(context.Background(), fixture.execution.ID, "successor", nil, true)
	require.NoError(t, err)
	require.Equal(t, uint64(2), receiveDispatchPromptGeneration(t, fixture.promptGenerations))
	require.True(t, fixture.execution.dispatchedPromptPending.Load(),
		"the successor dispatch must own its pending barrier")

	fixture.execution.messageMu.Lock()
	fixture.execution.messageBuffer.WriteString("successor transcript")
	fixture.execution.messageMu.Unlock()
	require.False(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
		Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
	}), "a stale predecessor completion must not claim the successor")
	require.False(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
		Type: "complete", SessionID: fixture.execution.SessionID,
	}), "an unnumbered completion must remain rejected while the successor is pending")

	fixture.execution.messageMu.Lock()
	transcript := fixture.execution.messageBuffer.String()
	fixture.execution.messageMu.Unlock()
	require.Equal(t, "successor transcript", transcript,
		"stale and unnumbered events must not flush successor transcript state")
	require.True(t, fixture.execution.dispatchedPromptPending.Load(),
		"stale and unnumbered events must not release the successor barrier")
	select {
	case signal := <-fixture.execution.promptDoneCh:
		t.Fatalf("rejected completion enqueued a signal: %+v", signal)
	default:
	}
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.5
func TestDispatchCompletion_FinalizationBarrier(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	flushStarted := make(chan struct{})
	flushFinished := make(chan struct{})
	releaseFlush := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFlush) }) }
	t.Cleanup(release)
	var flushedText string
	stream := newStreamCoalescer(time.Hour, func(chunk coalescedStreamChunk) {
		flushedText = chunk.content
		close(flushStarted)
		<-releaseFlush
		close(flushFinished)
	})
	stream.mu.Lock()
	stream.lastEventType = "message_streaming"
	stream.lastMessageID = "predecessor-message"
	stream.lastPromptGeneration = 1
	stream.mu.Unlock()
	fixture.execution.streamMu.Lock()
	fixture.execution.stream = stream
	fixture.execution.streamMu.Unlock()
	stream.add(coalescedStreamChunk{
		eventType: "message_streaming", messageID: "predecessor-message",
		content: "predecessor transcript tail", promptGeneration: 1, isAppend: true,
	})

	promptAdmitted := make(chan struct{})
	fixture.manager.sessionManager.beforePromptDispatchHook = func() { close(promptAdmitted) }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	successorResult := make(chan error, 1)
	go func() {
		_, err := fixture.manager.PromptAgent(ctx, fixture.execution.ID, "successor", nil, true)
		successorResult <- err
	}()
	waitForDispatchPromptLock(t, fixture.execution)

	completionResult := make(chan bool, 1)
	go func() {
		completionResult <- fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
		})
	}()
	select {
	case <-flushStarted:
	case <-time.After(time.Second):
		t.Fatal("completion did not enter the blocked transcript flush")
	}

	require.True(t, fixture.execution.dispatchedPromptPending.Load(),
		"the barrier must remain set until predecessor transcript processing finishes")
	select {
	case <-promptAdmitted:
		t.Fatal("successor passed the dispatch barrier during predecessor finalization")
	default:
	}
	select {
	case generation := <-fixture.promptGenerations:
		t.Fatalf("successor generation %d reached agentctl before transcript flush", generation)
	default:
	}
	select {
	case signal := <-fixture.execution.promptDoneCh:
		t.Fatalf("completion signal was published before transcript flush: %+v", signal)
	default:
	}

	release()
	select {
	case accepted := <-completionResult:
		require.True(t, accepted)
	case <-time.After(time.Second):
		t.Fatal("completion did not finish after releasing the transcript flush")
	}
	select {
	case err := <-successorResult:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatalf("successor prompt did not complete: %v", ctx.Err())
	}
	require.Equal(t, uint64(2), receiveDispatchPromptGeneration(t, fixture.promptGenerations))
	select {
	case <-flushFinished:
	default:
		t.Fatal("successor reached agentctl before transcript processing finished")
	}
	require.Equal(t, "predecessor transcript tail", flushedText)
}

func waitForDispatchPromptLock(t *testing.T, execution *AgentExecution) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !execution.promptMu.TryLock() {
			runtime.Gosched()
			return
		}
		execution.promptMu.Unlock()
		runtime.Gosched()
	}
	t.Fatal("successor did not acquire prompt serialization before completion")
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.6
func TestDispatchCompletion_ErrorReleasesOnlyMatchingGeneration(t *testing.T) {
	t.Run("matching_error_releases_barrier", func(t *testing.T) {
		fixture := newDispatchCompletionFixture(t, false)
		require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
			Error: "provider failed", Data: map[string]any{"is_error": true},
		}))

		require.False(t, fixture.execution.dispatchedPromptPending.Load(),
			"a matching terminal error must release its completed prompt barrier")
		require.Equal(t, v1.AgentStatusFailed, fixture.execution.Status,
			"releasing the barrier must preserve failure handling")
		select {
		case signal := <-fixture.execution.promptDoneCh:
			require.Equal(t, uint64(1), signal.PromptGeneration)
			require.True(t, signal.IsError)
		case <-time.After(time.Second):
			t.Fatal("terminal error did not preserve its prompt completion signal")
		}
	})

	t.Run("stale_error_preserves_successor", func(t *testing.T) {
		fixture := newDispatchCompletionFixture(t, false)
		require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
		}))
		_, err := fixture.manager.PromptAgent(context.Background(), fixture.execution.ID, "successor", nil, true)
		require.NoError(t, err)
		require.Equal(t, uint64(2), receiveDispatchPromptGeneration(t, fixture.promptGenerations))

		fixture.execution.messageMu.Lock()
		fixture.execution.messageBuffer.WriteString("successor transcript")
		fixture.execution.messageMu.Unlock()
		require.False(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
			Error: "stale provider error", Data: map[string]any{"is_error": true},
		}), "a stale terminal error must not claim the current generation")

		fixture.execution.messageMu.Lock()
		transcript := fixture.execution.messageBuffer.String()
		fixture.execution.messageMu.Unlock()
		require.Equal(t, "successor transcript", transcript)
		require.Equal(t, v1.AgentStatusRunning, fixture.execution.Status)
		require.True(t, fixture.execution.dispatchedPromptPending.Load(),
			"a stale error must not release the successor dispatch barrier")
		select {
		case signal := <-fixture.execution.promptDoneCh:
			t.Fatalf("stale error enqueued a completion signal: %+v", signal)
		default:
		}
	})
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.3, AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.6
func TestDispatchCompletion_ErrorBeforeAcknowledgementReleasesDispatchGuard(t *testing.T) {
	guard := sync.Mutex{}
	guard.Lock()
	guardState := 1
	var releaseGuardOnce sync.Once
	releaseGuard := func() {
		releaseGuardOnce.Do(func() {
			guardState = 0
			guard.Unlock()
		})
	}
	flushStarted := make(chan struct{})
	releaseFlush := make(chan struct{})
	var releaseFlushOnce sync.Once
	finishFlush := func() { releaseFlushOnce.Do(func() { close(releaseFlush) }) }
	t.Cleanup(finishFlush)
	failurePublishStarted := make(chan struct{})
	var publishOnce sync.Once
	fixture := setupDispatchCompletionFixture(t, func(fixture *dispatchCompletionFixture) {
		eventBus, ok := fixture.manager.eventPublisher.eventBus.(*MockEventBus)
		require.True(t, ok)
		eventBus.OnPublish = func(subject string, _ *bus.Event) {
			if subject != events.AgentFailed {
				return
			}
			publishOnce.Do(func() { close(failurePublishStarted) })
			guard.Lock()
			guardState = 2
			guard.Unlock()
		}
	}, func(fixture *dispatchCompletionFixture, generation uint64) {
		if generation != 1 {
			return
		}
		stream := newStreamCoalescer(time.Hour, func(coalescedStreamChunk) {
			close(flushStarted)
			<-releaseFlush
		})
		stream.mu.Lock()
		stream.lastEventType = "message_streaming"
		stream.lastMessageID = "failing-message"
		stream.lastPromptGeneration = generation
		stream.mu.Unlock()
		fixture.execution.streamMu.Lock()
		fixture.execution.stream = stream
		fixture.execution.streamMu.Unlock()
		stream.add(coalescedStreamChunk{
			eventType: "message_streaming", messageID: "failing-message",
			content: "failing response", promptGeneration: generation, isAppend: true,
		})
		go func() {
			fixture.completionAccepted <- fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
				Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: generation,
				Error: "provider failed", Data: map[string]any{"is_error": true},
			})
		}()
		select {
		case <-flushStarted:
			finishFlush()
		case <-time.After(time.Second):
			return
		}
		select {
		case <-failurePublishStarted:
		case <-time.After(time.Second):
		}
	})

	promptResult := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		_, err := fixture.manager.sessionManager.SendPromptWithDispatchCallback(
			ctx, fixture.execution, "dispatch-only", true, nil, true, releaseGuard,
		)
		promptResult <- err
	}()

	var promptErr error
	deadlocked := false
	select {
	case promptErr = <-promptResult:
	case <-time.After(500 * time.Millisecond):
		deadlocked = true
		releaseGuard()
		select {
		case promptErr = <-promptResult:
		case <-time.After(time.Second):
			t.Fatalf("prompt dispatch did not unwind after releasing the simulated guard; actions=%v publish_started=%v", fixture.mock.getActionLog(), channelClosed(failurePublishStarted))
		}
	}
	require.False(t, deadlocked,
		"a synchronous failure subscriber must not prevent acknowledgement from releasing its guard")
	require.NoError(t, promptErr)
	require.True(t, receiveDispatchCompletion(t, fixture.completionAccepted), "the matching error must be accepted")
	require.Equal(t, v1.AgentStatusFailed, fixture.execution.Status)
	guard.Lock()
	require.Equal(t, 2, guardState, "the synchronous failure subscriber must acquire the released dispatch guard")
	guard.Unlock()
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.4, AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.5
func TestDispatchCompletion_ErrorFinalizationRejectsWaitingSuccessor(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	flushStarted := make(chan struct{})
	releaseFlush := make(chan struct{})
	var releaseFlushOnce sync.Once
	releaseFlushFn := func() { releaseFlushOnce.Do(func() { close(releaseFlush) }) }
	t.Cleanup(releaseFlushFn)
	stream := newStreamCoalescer(time.Hour, func(coalescedStreamChunk) {
		close(flushStarted)
		<-releaseFlush
	})
	stream.mu.Lock()
	stream.lastEventType = "message_streaming"
	stream.lastMessageID = "failing-message"
	stream.lastPromptGeneration = 1
	stream.mu.Unlock()
	fixture.execution.streamMu.Lock()
	fixture.execution.stream = stream
	fixture.execution.streamMu.Unlock()
	stream.add(coalescedStreamChunk{
		eventType: "message_streaming", messageID: "failing-message",
		content: "failing response", promptGeneration: 1, isAppend: true,
	})

	completionResult := make(chan bool, 1)
	go func() {
		completionResult <- fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
			Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
			Error: "provider failed", Data: map[string]any{"is_error": true},
		})
	}()
	select {
	case <-flushStarted:
	case <-time.After(time.Second):
		t.Fatal("error finalization did not reach the transcript barrier")
	}

	// Keep terminal persistence from applying FAILED while the accepted error
	// completion releases its waiter. This exposes a successor that validated
	// RUNNING before blocking on promptLifecycleMu.
	fixture.manager.executionStore.mu.RLock()
	var releaseStoreReadOnce sync.Once
	releaseStoreRead := func() { releaseStoreReadOnce.Do(fixture.manager.executionStore.mu.RUnlock) }
	t.Cleanup(releaseStoreRead)
	releaseFlushFn()
	require.Equal(t, v1.AgentStatusRunning, fixture.execution.Status)

	// The vulnerable ordering clears this flag before attempting the terminal
	// store write. Holding the read lock makes that gap stable and observable.
	wasReleasedBeforeFailure := waitForDispatchPendingValue(t, fixture.execution, false, 500*time.Millisecond)
	admissionReached := make(chan struct{}, 1)
	successorResult := make(chan error, 1)
	go func() {
		_, err := fixture.manager.sessionManager.SendPromptWithAdmissionCallback(
			ctx, fixture.execution, "successor", true, nil, true,
			func() error {
				admissionReached <- struct{}{}
				return nil
			}, nil,
		)
		successorResult <- err
	}()
	if wasReleasedBeforeFailure {
		select {
		case <-admissionReached:
		case <-time.After(time.Second):
			select {
			case err := <-successorResult:
				t.Fatalf("successor returned before admission: %v; status=%s actions=%v", err, fixture.execution.Status, fixture.mock.getActionLog())
			default:
				t.Fatalf("successor did not reach admission; status=%s actions=%v", fixture.execution.Status, fixture.mock.getActionLog())
			}
		}
	}
	select {
	case generation := <-fixture.promptGenerations:
		t.Fatalf("successor generation %d reached agentctl before failed-state persistence", generation)
	default:
	}
	releaseStoreRead()

	select {
	case accepted := <-completionResult:
		require.True(t, accepted)
	case <-ctx.Done():
		t.Fatalf("error completion did not finish: %v", ctx.Err())
	}
	select {
	case err := <-successorResult:
		require.Error(t, err, "a successor must not reopen a failed execution")
	case <-ctx.Done():
		t.Fatalf("successor prompt did not settle: %v", ctx.Err())
	}
	require.Equal(t, v1.AgentStatusFailed, fixture.execution.Status)
	require.Equal(t, uint64(1), fixture.execution.promptGenerationSnapshot(),
		"failure finalization must not admit a new prompt generation")
	select {
	case generation := <-fixture.promptGenerations:
		t.Fatalf("successor generation %d reached agentctl on a failed execution", generation)
	default:
	}
}

func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func waitForDispatchPendingValue(
	t *testing.T,
	execution *AgentExecution,
	want bool,
	timeout time.Duration,
) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if execution.dispatchedPromptPending.Load() == want {
			return true
		}
		runtime.Gosched()
	}
	return execution.dispatchedPromptPending.Load() == want
}

// @covers AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.2, AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.5
func TestDispatchCompletion_NextPromptUsesSameExecution(t *testing.T) {
	fixture := newDispatchCompletionFixture(t, false)
	require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
		Type: "complete", SessionID: fixture.execution.SessionID, PromptGeneration: 1,
	}))
	fixture.manager.handleAgentEvent(fixture.execution, agentctl.AgentEvent{
		Type: "message_chunk", Text: "autonomous response",
	})
	require.True(t, fixture.manager.handleCompleteEvent(fixture.execution, &agentctl.AgentEvent{
		Type: "complete", SessionID: fixture.execution.SessionID,
	}), "the autonomous completion must be accepted before successor dispatch")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	_, err := fixture.manager.PromptAgent(ctx, fixture.execution.ID, "next prompt", nil, true)
	require.NoError(t, err)
	require.Equal(t, uint64(2), receiveDispatchPromptGeneration(t, fixture.promptGenerations))

	current, exists := fixture.manager.executionStore.GetBySessionID(fixture.execution.SessionID)
	require.True(t, exists)
	require.Same(t, fixture.execution, current,
		"the eligible successor must use the existing execution")
	actions := fixture.mock.getActionLog()
	promptCount := 0
	for _, action := range actions {
		if action == "agent.prompt" {
			promptCount++
		}
	}
	require.Equal(t, 2, promptCount, "exactly one successor prompt must reach agentctl")
	for _, action := range fixture.mock.getHTTPActionLog() {
		require.NotEqual(t, "stop", strings.ToLower(action), "the execution must not be stopped or restarted")
		require.NotEqual(t, "start", strings.ToLower(action), "the execution must not be stopped or restarted")
	}
}
