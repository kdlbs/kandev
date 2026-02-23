package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MockEventBusWithTracking provides detailed tracking of published events for testing
type MockEventBusWithTracking struct {
	PublishedEvents []trackedEvent
	mu              sync.Mutex
}

type trackedEvent struct {
	Subject string
	Event   *bus.Event
}

func (m *MockEventBusWithTracking) Publish(ctx context.Context, subject string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PublishedEvents = append(m.PublishedEvents, trackedEvent{Subject: subject, Event: event})
	return nil
}

func (m *MockEventBusWithTracking) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBusWithTracking) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBusWithTracking) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBusWithTracking) Close() {}

func (m *MockEventBusWithTracking) IsConnected() bool {
	return true
}

func (m *MockEventBusWithTracking) getStreamEvents() []AgentStreamEventPayload {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []AgentStreamEventPayload
	for _, te := range m.PublishedEvents {
		if payload, ok := te.Event.Data.(AgentStreamEventPayload); ok {
			result = append(result, payload)
		}
	}
	return result
}

// createTestManagerWithTracking creates a manager with a tracking event bus for testing
func createTestManagerWithTracking() (*Manager, *MockEventBusWithTracking) {
	log := newTestLogger()
	reg := newTestRegistry()
	eventBus := &MockEventBusWithTracking{}
	credsMgr := &MockCredentialsManager{}
	profileResolver := &MockProfileResolver{}
	mgr := NewManager(reg, eventBus, nil, nil, credsMgr, profileResolver, nil, ExecutorFallbackWarn, log)
	return mgr, eventBus
}

// createTestExecution creates a test execution with proper initialization
func createTestExecution(id, taskID, sessionID string) *AgentExecution {
	return &AgentExecution{
		ID:           id,
		TaskID:       taskID,
		SessionID:    sessionID,
		Status:       v1.AgentStatusRunning,
		StartedAt:    time.Now(),
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
}

// TestHandleAgentEvent_StreamingThenComplete tests the normal flow:
// message_chunk events followed by complete event - should NOT create duplicate
func TestHandleAgentEvent_StreamingThenComplete(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Simulate streaming chunks with newlines (which trigger publishing)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Hello, world!\n",
	})

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "This is a test.\n",
	})

	// Now send complete event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	// Verify: streaming was used, so complete should NOT have text
	events := eventBus.getStreamEvents()

	// Count message_streaming events (creates/appends)
	var messageStreamingEvents []AgentStreamEventPayload
	var completeEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil {
			switch e.Data.Type {
			case "message_streaming":
				messageStreamingEvents = append(messageStreamingEvents, e)
			case "complete":
				completeEvents = append(completeEvents, e)
			}
		}
	}

	// Should have streaming messages
	if len(messageStreamingEvents) == 0 {
		t.Error("expected message_streaming events, got none")
	}

	// Should have exactly one complete event
	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	// The complete event should NOT have text (streaming handled it via buffer)
	if len(completeEvents) > 0 && completeEvents[0].Data.Text != "" {
		t.Errorf("complete event should not have text when streaming was used, got %q", completeEvents[0].Data.Text)
	}
}

// TestHandleAgentEvent_StreamingThenToolCallThenComplete tests the scenario that could cause duplicates:
// message_chunk → tool_call (clears currentMessageID) → complete
// This verifies that the buffer is properly flushed on complete after tool calls
func TestHandleAgentEvent_StreamingThenToolCallThenComplete(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Simulate streaming chunks
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Let me check that for you.\n",
	})

	// Verify currentMessageID is set after message_chunk
	execution.messageMu.Lock()
	msgIDBeforeToolCall := execution.currentMessageID
	execution.messageMu.Unlock()

	if msgIDBeforeToolCall == "" {
		t.Error("currentMessageID should be set after message_chunk")
	}

	// Tool call - this flushes buffer and clears currentMessageID
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})

	// After tool call, currentMessageID should be cleared
	execution.messageMu.Lock()
	msgIDAfterToolCall := execution.currentMessageID
	execution.messageMu.Unlock()

	if msgIDAfterToolCall != "" {
		t.Errorf("currentMessageID should be cleared after tool_call, got %q", msgIDAfterToolCall)
	}

	// Now complete - this should flush the buffer
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	// Find the complete event
	var completeEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "complete" {
			completeEvents = append(completeEvents, e)
		}
	}

	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	// The complete event should NOT have text (streaming was used)
	if len(completeEvents) > 0 && completeEvents[0].Data.Text != "" {
		t.Errorf("complete event should not have text when streaming was used (even after tool_call), got %q",
			completeEvents[0].Data.Text)
	}
}

// TestHandleAgentEvent_CompleteWithoutStreaming verifies that complete events are
// properly handled when no streaming was used (buffer is empty).
// All adapters now send text via message_chunk events, so this tests the empty buffer case.
func TestHandleAgentEvent_CompleteWithoutStreaming(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Complete event without any prior streaming (e.g., agent did only tool calls)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	var completeEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "complete" {
			completeEvents = append(completeEvents, e)
		}
	}

	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	// Complete event should be published successfully
	// The buffer was empty, so no message_streaming events should be generated
	var streamingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "message_streaming" {
			streamingEvents = append(streamingEvents, e)
		}
	}

	if len(streamingEvents) != 0 {
		t.Errorf("expected 0 message_streaming events when buffer is empty, got %d", len(streamingEvents))
	}
}

// TestHandleAgentEvent_CompleteWithBufferedText verifies that buffered text
// without streaming is emitted as a final streaming event on complete.
func TestHandleAgentEvent_CompleteWithBufferedText(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Buffer text without newlines (no streaming event should be emitted)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Final message without newline",
	})

	// Complete event should flush buffer into a streaming event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	var completeEvents []AgentStreamEventPayload
	var streamingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil {
			switch e.Data.Type {
			case "complete":
				completeEvents = append(completeEvents, e)
			case "message_streaming":
				streamingEvents = append(streamingEvents, e)
			}
		}
	}

	if len(completeEvents) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(completeEvents))
	}

	if len(completeEvents) > 0 && completeEvents[0].Data.Text != "" {
		t.Errorf("expected complete event to have empty text, got %q", completeEvents[0].Data.Text)
	}

	if len(streamingEvents) != 1 {
		t.Errorf("expected 1 message_streaming event when no newlines, got %d", len(streamingEvents))
	} else if streamingEvents[0].Data.Text != "Final message without newline" {
		t.Errorf("expected streaming event to carry buffered text, got %q", streamingEvents[0].Data.Text)
	}
}

// TestHandleAgentEvent_CompleteThenMessageChunk tests the scenario where
// message_chunk arrives after complete. This documents the behavior when
// an adapter incorrectly sends text after the turn has completed.
// With the new architecture, adapters should NOT send message_chunk after complete.
func TestHandleAgentEvent_CompleteThenMessageChunk(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// First, simulate normal streaming during the turn
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Processing your request...\n",
	})

	// Complete event arrives - this flushes the buffer
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	// Now message_chunk arrives AFTER complete
	// This shouldn't happen with properly implemented adapters,
	// but we document the behavior: it creates a new message
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Done!\n",
	})

	events := eventBus.getStreamEvents()

	// Count message_streaming events
	var messageStreamingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "message_streaming" {
			messageStreamingEvents = append(messageStreamingEvents, e)
		}
	}

	// Document the behavior: message_chunk after complete starts a new message
	t.Logf("Got %d message_streaming events", len(messageStreamingEvents))
	for i, e := range messageStreamingEvents {
		t.Logf("  Event %d: MessageID=%s, IsAppend=%v, Text=%q",
			i, e.Data.MessageID, e.Data.IsAppend, e.Data.Text)
	}

	// The second message_chunk (after complete) should start a NEW message
	// since currentMessageID was cleared by the complete event
	if len(messageStreamingEvents) >= 2 {
		lastEvent := messageStreamingEvents[len(messageStreamingEvents)-1]
		if !lastEvent.Data.IsAppend {
			t.Log("Expected behavior: message_chunk after complete creates a new message")
		}
	}
}

// TestHandleAgentEvent_MultipleToolCalls tests streaming → tool → streaming → tool → complete
func TestHandleAgentEvent_MultipleToolCalls(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Message before first tool
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Let me read the file.\n",
	})

	// First tool call
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})

	// Tool update (complete)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_update",
		ToolCallID: "tool-1",
		ToolStatus: "complete",
	})

	// Message after first tool
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Now let me modify it.\n",
	})

	// Second tool call
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-2",
		ToolName:   "write_file",
	})

	// Tool update (complete)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_update",
		ToolCallID: "tool-2",
		ToolStatus: "complete",
	})

	// Final message
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Done with both tasks!\n",
	})

	// Complete
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	events := eventBus.getStreamEvents()

	// Count different event types
	var messageStreamingCount, toolCallCount, completeCount int
	for _, e := range events {
		if e.Data != nil {
			switch e.Data.Type {
			case "message_streaming":
				messageStreamingCount++
			case "tool_call":
				toolCallCount++
			case "complete":
				completeCount++
			}
		}
	}

	t.Logf("Events: message_streaming=%d, tool_call=%d, complete=%d",
		messageStreamingCount, toolCallCount, completeCount)

	// Should have multiple streaming messages (one per "segment" before tool calls)
	if messageStreamingCount < 3 {
		t.Errorf("expected at least 3 message_streaming events for 3 message segments, got %d", messageStreamingCount)
	}

	if toolCallCount != 2 {
		t.Errorf("expected 2 tool_call events, got %d", toolCallCount)
	}

	if completeCount != 1 {
		t.Errorf("expected 1 complete event, got %d", completeCount)
	}

	// Find the complete event and verify it has no text
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "complete" {
			if e.Data.Text != "" {
				t.Errorf("complete event should not have text when streaming was used, got %q", e.Data.Text)
			}
		}
	}
}

// TestHandleAgentEvent_CompleteSignalsPromptDoneCh verifies that the complete event
// signals the promptDoneCh channel with the correct stop reason.
func TestHandleAgentEvent_CompleteSignalsPromptDoneCh(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Send a normal complete event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	// promptDoneCh should have a signal
	select {
	case signal := <-execution.promptDoneCh:
		if signal.IsError {
			t.Error("expected non-error signal for normal complete")
		}
		if signal.StopReason != "end_turn" {
			t.Errorf("expected stop_reason 'end_turn', got %q", signal.StopReason)
		}
	default:
		t.Error("expected signal on promptDoneCh, got none")
	}
}

// TestHandleAgentEvent_ErrorCompleteSignalsPromptDoneCh verifies that an error completion
// signals the promptDoneCh channel with IsError=true and the error message.
func TestHandleAgentEvent_ErrorCompleteSignalsPromptDoneCh(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Send an error complete event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:  "complete",
		Error: "something went wrong",
		Data:  map[string]interface{}{"is_error": true},
	})

	// promptDoneCh should have an error signal
	select {
	case signal := <-execution.promptDoneCh:
		if !signal.IsError {
			t.Error("expected error signal for error complete")
		}
		if signal.StopReason != "error" {
			t.Errorf("expected stop_reason 'error', got %q", signal.StopReason)
		}
		if signal.Error != "something went wrong" {
			t.Errorf("expected error 'something went wrong', got %q", signal.Error)
		}
	default:
		t.Error("expected signal on promptDoneCh, got none")
	}
}

// TestHandleAgentEvent_UpdatesLastActivityAt verifies that every agent event
// updates the lastActivityAt timestamp for stall detection.
func TestHandleAgentEvent_UpdatesLastActivityAt(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Set lastActivityAt to a known old time
	oldTime := time.Now().Add(-10 * time.Minute)
	execution.lastActivityAtMu.Lock()
	execution.lastActivityAt = oldTime
	execution.lastActivityAtMu.Unlock()

	// Send any event
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "hello\n",
	})

	// lastActivityAt should be updated to approximately now
	execution.lastActivityAtMu.Lock()
	elapsed := time.Since(execution.lastActivityAt)
	execution.lastActivityAtMu.Unlock()

	if elapsed > 1*time.Second {
		t.Errorf("lastActivityAt not updated: elapsed %v since last event", elapsed)
	}
}

// TestHandleAgentEvent_PromptDoneChDoesNotBlockWhenFull verifies that signaling
// promptDoneCh with a full channel (no receiver) doesn't block the event handler.
func TestHandleAgentEvent_PromptDoneChDoesNotBlockWhenFull(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Pre-fill the channel
	execution.promptDoneCh <- PromptCompletionSignal{StopReason: "stale"}

	// Send complete event — should not block
	done := make(chan struct{})
	go func() {
		mgr.handleAgentEvent(execution, agentctl.AgentEvent{
			Type: "complete",
		})
		close(done)
	}()

	select {
	case <-done:
		// Good — didn't block
	case <-time.After(2 * time.Second):
		t.Fatal("handleAgentEvent blocked when promptDoneCh was full")
	}
}

// TestHandleAgentEvent_ReasoningThenToolCall tests the scenario where thinking content
// accumulates without newlines (no streaming triggered), then a tool_call flushes the
// thinking buffer. The fix in flushMessageBuffer must clear currentThinkingID after
// publishStreamingThinking sets it as a side effect, so the next thinking segment starts fresh.
func TestHandleAgentEvent_ReasoningThenToolCall(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Reasoning without newlines — stays in buffer, no streaming emitted yet
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:          "reasoning",
		ReasoningText: "thinking without newline",
	})

	// currentThinkingID should still be empty (no streaming happened yet)
	execution.messageMu.Lock()
	idBeforeToolCall := execution.currentThinkingID
	execution.messageMu.Unlock()
	if idBeforeToolCall != "" {
		t.Errorf("currentThinkingID should be empty before flush, got %q", idBeforeToolCall)
	}

	// Tool call flushes the thinking buffer via flushMessageBuffer;
	// publishStreamingThinking will set currentThinkingID as a side effect,
	// then the fix must clear it.
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})

	// After the tool_call flush, currentThinkingID must be empty
	execution.messageMu.Lock()
	idAfterToolCall := execution.currentThinkingID
	execution.messageMu.Unlock()
	if idAfterToolCall != "" {
		t.Errorf("currentThinkingID should be cleared after tool_call flush, got %q", idAfterToolCall)
	}

	// A thinking_streaming event should have been published for the buffered content
	events := eventBus.getStreamEvents()
	var thinkingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "thinking_streaming" {
			thinkingEvents = append(thinkingEvents, e)
		}
	}
	if len(thinkingEvents) == 0 {
		t.Error("expected thinking_streaming event to be published during tool_call flush")
	}
}

// TestHandleAgentEvent_ReasoningThenComplete tests that thinking content accumulated
// without newlines is flushed on complete, and currentThinkingID is cleared afterwards
// so a subsequent turn starts with a fresh thinking message.
func TestHandleAgentEvent_ReasoningThenComplete(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Reasoning without newlines — no streaming
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:          "reasoning",
		ReasoningText: "brief thought",
	})

	// Complete flushes the buffer
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
	})

	// currentThinkingID must be empty after flush
	execution.messageMu.Lock()
	idAfterComplete := execution.currentThinkingID
	execution.messageMu.Unlock()
	if idAfterComplete != "" {
		t.Errorf("currentThinkingID should be cleared after complete flush, got %q", idAfterComplete)
	}

	// A thinking_streaming event should have been published
	events := eventBus.getStreamEvents()
	var thinkingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "thinking_streaming" {
			thinkingEvents = append(thinkingEvents, e)
		}
	}
	if len(thinkingEvents) == 0 {
		t.Error("expected thinking_streaming event to be published during complete flush")
	}
}

// TestHandleAgentEvent_ReasoningWithNewlinesThenToolCall tests that reasoning content
// with newlines triggers streaming (sets currentThinkingID), a tool_call then flushes
// any remainder and clears currentThinkingID so the next thinking segment is a new message.
func TestHandleAgentEvent_ReasoningWithNewlinesThenToolCall(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	// Reasoning with newline — triggers streaming, sets currentThinkingID
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:          "reasoning",
		ReasoningText: "step one\n",
	})

	execution.messageMu.Lock()
	idAfterStreaming := execution.currentThinkingID
	execution.messageMu.Unlock()
	if idAfterStreaming == "" {
		t.Error("currentThinkingID should be set after streamed reasoning chunk")
	}

	// Tool call flushes (empty remainder) and clears the ID
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})

	execution.messageMu.Lock()
	idAfterToolCall := execution.currentThinkingID
	execution.messageMu.Unlock()
	if idAfterToolCall != "" {
		t.Errorf("currentThinkingID should be cleared after tool_call, got %q", idAfterToolCall)
	}

	// Now reasoning again after tool_call — should start a NEW thinking message (IsAppend=false)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:          "reasoning",
		ReasoningText: "step two\n",
	})

	events := eventBus.getStreamEvents()
	var thinkingEvents []AgentStreamEventPayload
	for _, e := range events {
		if e.Data != nil && e.Data.Type == "thinking_streaming" {
			thinkingEvents = append(thinkingEvents, e)
		}
	}

	// Should have at least 2 thinking_streaming events: one before and one after tool_call
	if len(thinkingEvents) < 2 {
		t.Errorf("expected at least 2 thinking_streaming events, got %d", len(thinkingEvents))
	}

	// The second thinking segment must start a new message (IsAppend=false)
	lastEvent := thinkingEvents[len(thinkingEvents)-1]
	if lastEvent.Data.IsAppend {
		t.Errorf("reasoning chunk after tool_call should start a new thinking message, but got IsAppend=true")
	}
}
