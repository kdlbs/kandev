package lifecycle

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
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
	mgr := NewManager(reg, eventBus, nil, credsMgr, profileResolver, nil, ExecutorFallbackWarn, "", log)
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

func TestHandleAgentEvent_UserMessageChunkNotBufferedAsAssistant(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "<hidden-system-prompt>\nhello",
		Role: "user",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Hello.",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete"})

	var streamedText string
	for _, event := range eventBus.getStreamEvents() {
		if event.Data != nil && event.Data.Type == "message_streaming" {
			streamedText += event.Data.Text
		}
	}
	if streamedText != "Hello." {
		t.Fatalf("streamed assistant text = %q, want %q", streamedText, "Hello.")
	}
}

func TestHandleAgentEvent_ProtocolMessageResumesAcrossToolCall(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "message_chunk",
		Text:              "before tool",
		ProtocolMessageID: "acp-message-1",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "message_chunk",
		Text:              "after tool",
		ProtocolMessageID: "acp-message-1",
	})

	messageEvents := streamEventsOfType(eventBus, "message_streaming")
	if len(messageEvents) != 2 {
		t.Fatalf("message event count = %d, want 2", len(messageEvents))
	}
	first, second := messageEvents[0].Data, messageEvents[1].Data
	if first.MessageID == "" || second.MessageID != first.MessageID {
		t.Fatalf("Kandev message IDs = (%q, %q), want same non-empty ID", first.MessageID, second.MessageID)
	}
	if first.IsAppend || !second.IsAppend {
		t.Fatalf("append flags = (%t, %t), want (false, true)", first.IsAppend, second.IsAppend)
	}
}

func TestHandleAgentEvent_InterleavedProtocolMessagesRemainDistinct(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: "message_chunk", Text: "a1", ProtocolMessageID: "message-a"},
		{Type: "message_chunk", Text: "b1", ProtocolMessageID: "message-b"},
		{Type: "message_chunk", Text: "a2", ProtocolMessageID: "message-a"},
	} {
		mgr.handleAgentEvent(execution, event)
	}

	messageEvents := streamEventsOfType(eventBus, "message_streaming")
	if len(messageEvents) != 3 {
		t.Fatalf("message event count = %d, want 3", len(messageEvents))
	}
	first, second, third := messageEvents[0].Data, messageEvents[1].Data, messageEvents[2].Data
	if first.Text != "a1" || second.Text != "b1" || third.Text != "a2" {
		t.Fatalf("event order = (%q, %q, %q), want (a1, b1, a2)", first.Text, second.Text, third.Text)
	}
	if first.MessageID == second.MessageID || third.MessageID != first.MessageID {
		t.Fatalf("Kandev message IDs = (%q, %q, %q), want A != B and A stable",
			first.MessageID, second.MessageID, third.MessageID)
	}
	if first.IsAppend || second.IsAppend || !third.IsAppend {
		t.Fatalf("append flags = (%t, %t, %t), want (false, false, true)",
			first.IsAppend, second.IsAppend, third.IsAppend)
	}
}

func TestHandleAgentEvent_ProtocolThoughtAndMessageIDsUseSeparateNamespaces(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "message_chunk",
		Text:              "answer",
		ProtocolMessageID: "shared-source-id",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "reasoning",
		ReasoningText:     "thought",
		ProtocolMessageID: "shared-source-id",
	})

	messageEvents := streamEventsOfType(eventBus, "message_streaming")
	thinkingEvents := streamEventsOfType(eventBus, "thinking_streaming")
	if len(messageEvents) != 1 || len(thinkingEvents) != 1 {
		t.Fatalf("event counts = message:%d thinking:%d, want 1 each", len(messageEvents), len(thinkingEvents))
	}
	if messageEvents[0].Data.MessageID == thinkingEvents[0].Data.MessageID {
		t.Fatalf("assistant and thought mapped to the same Kandev ID %q", messageEvents[0].Data.MessageID)
	}
}

func TestHandleAgentEvent_CompleteClearsProtocolMessageCorrelation(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "message_chunk",
		Text:              "turn one",
		ProtocolMessageID: "reused-source-id",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete"})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "message_chunk",
		Text:              "turn two",
		ProtocolMessageID: "reused-source-id",
	})

	messageEvents := streamEventsOfType(eventBus, "message_streaming")
	if len(messageEvents) != 2 {
		t.Fatalf("message event count = %d, want 2", len(messageEvents))
	}
	if messageEvents[0].Data.MessageID == messageEvents[1].Data.MessageID {
		t.Fatalf("protocol correlation leaked across completion: Kandev ID %q was reused",
			messageEvents[0].Data.MessageID)
	}
	if messageEvents[1].Data.IsAppend {
		t.Fatal("first chunk after completion was marked as append")
	}
}

func TestHandleAgentEvent_LegacyAssistantFlushesBeforeProtocolMessage(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "legacy first"})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "message_chunk",
		Text:              "protocol second",
		ProtocolMessageID: "protocol-message",
	})

	events := streamEventsOfType(eventBus, "message_streaming")
	if len(events) != 2 {
		t.Fatalf("message event count = %d, want 2", len(events))
	}
	if events[0].Data.Text != "legacy first" || events[1].Data.Text != "protocol second" {
		t.Fatalf("message order = (%q, %q), want legacy then protocol",
			events[0].Data.Text, events[1].Data.Text)
	}
	if events[0].Data.MessageID == events[1].Data.MessageID {
		t.Fatalf("mixed legacy and protocol chunks shared Kandev ID %q", events[0].Data.MessageID)
	}
}

func TestHandleAgentEvent_ProtocolToLegacyTransitionCreatesBoundary(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: "message_chunk", Text: "protocol first", ProtocolMessageID: "protocol-message"},
		{Type: "message_chunk", Text: "legacy second\n"},
		{Type: "message_chunk", Text: "protocol third", ProtocolMessageID: "protocol-message"},
		{Type: "message_chunk", Text: "legacy fourth\n"},
	} {
		mgr.handleAgentEvent(execution, event)
	}

	events := streamEventsOfType(eventBus, "message_streaming")
	if len(events) != 4 {
		t.Fatalf("message event count = %d, want 4", len(events))
	}
	texts := []string{events[0].Data.Text, events[1].Data.Text, events[2].Data.Text, events[3].Data.Text}
	wantTexts := []string{"protocol first", "legacy second\n", "protocol third", "legacy fourth\n"}
	for i := range wantTexts {
		if texts[i] != wantTexts[i] {
			t.Fatalf("event %d text = %q, want %q", i, texts[i], wantTexts[i])
		}
	}
	protocolID := events[0].Data.MessageID
	if events[2].Data.MessageID != protocolID || !events[2].Data.IsAppend {
		t.Fatalf("resumed protocol event = id:%q append:%t, want id:%q append:true",
			events[2].Data.MessageID, events[2].Data.IsAppend, protocolID)
	}
	if events[1].Data.MessageID == protocolID || events[3].Data.MessageID == protocolID {
		t.Fatal("legacy chunks merged into protocol record")
	}
	if events[1].Data.MessageID == events[3].Data.MessageID {
		t.Fatal("legacy chunks on opposite sides of a protocol chunk shared a record")
	}
}

func TestHandleAgentEvent_LegacyThinkingFlushesBeforeProtocolThinking(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "reasoning", ReasoningText: "legacy thought"})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "reasoning",
		ReasoningText:     "protocol thought",
		ProtocolMessageID: "thought-message",
	})

	events := streamEventsOfType(eventBus, "thinking_streaming")
	if len(events) != 2 {
		t.Fatalf("thinking event count = %d, want 2", len(events))
	}
	if events[0].Data.Text != "legacy thought" || events[1].Data.Text != "protocol thought" {
		t.Fatalf("thinking order = (%q, %q), want legacy then protocol",
			events[0].Data.Text, events[1].Data.Text)
	}
	if events[0].Data.MessageID == events[1].Data.MessageID {
		t.Fatalf("mixed legacy and protocol thinking shared Kandev ID %q", events[0].Data.MessageID)
	}
}

func TestHandleAgentEvent_ProtocolAssistantHistoryPersistedOnceInWireOrder(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: "message_chunk", Text: "first", ProtocolMessageID: "message-a"},
		{Type: "message_chunk", Text: " second", ProtocolMessageID: "message-b"},
		{Type: "message_chunk", Text: " third", ProtocolMessageID: "message-a"},
	} {
		mgr.handleAgentEvent(execution, event)
	}
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete"})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete"})

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	var agentEntries []HistoryEntry
	for _, entry := range entries {
		if entry.Type == "agent_message" {
			agentEntries = append(agentEntries, entry)
		}
	}
	if len(agentEntries) != 1 {
		t.Fatalf("agent history entry count = %d, want 1", len(agentEntries))
	}
	if agentEntries[0].Content != "first second third" {
		t.Fatalf("agent history content = %q, want wire-order transcript", agentEntries[0].Content)
	}
	execution.messageMu.Lock()
	historyBufferLen := execution.assistantHistoryBuffer.Len()
	execution.messageMu.Unlock()
	if historyBufferLen != 0 {
		t.Fatalf("assistant history accumulator length = %d after completion, want 0", historyBufferLen)
	}
}

func TestStreamDisconnect_PersistsPartialAssistantHistoryExactlyOnce(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              "message_chunk",
		Text:              "partial response",
		ProtocolMessageID: "message-a",
	})
	mgr.handleStreamDisconnect(execution, errors.New("connection lost"), 0)

	// A later prompt or reset must not discard or duplicate the segment.
	flushAssistantHistory(execution, history, mgr.logger)
	execution.messageMu.Lock()
	execution.resetStreamingStateLocked()
	execution.messageMu.Unlock()
	mgr.flushAssistantHistory(execution)

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 1 || entries[0].Type != "agent_message" ||
		entries[0].Content != "partial response" {
		t.Fatalf("disconnect history = %+v, want one partial assistant message", entries)
	}

	messageEvents := streamEventsOfType(eventBus, "message_streaming")
	if len(messageEvents) != 1 || messageEvents[0].Data.Text != "partial response" {
		t.Fatalf("visible messages = %+v, want the original chunk exactly once", messageEvents)
	}
}

func TestPromptResetWinsDisconnectRaceWithoutDroppingHistory(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "response before disconnect callback",
	})

	// connectUpdatesStream signals promptDoneCh before invoking the disconnect
	// callback. Model a next prompt claiming the buffer first.
	flushAssistantHistory(execution, history, mgr.logger)
	execution.messageMu.Lock()
	execution.resetStreamingStateLocked()
	execution.messageMu.Unlock()
	mgr.handleStreamDisconnect(execution, errors.New("connection lost"), 0)

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "response before disconnect callback" {
		t.Fatalf("disconnect-race history = %+v, want one partial assistant message", entries)
	}
}

func TestDelayedDisconnectCannotAffectReplacementGeneration(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	originalGeneration, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin original prompt: %v", err)
	}
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "old partial"})

	callbackReached := make(chan struct{})
	releaseCallback := make(chan struct{})
	callbackDone := make(chan struct{})
	streamManager := NewStreamManager(mgr.logger, StreamCallbacks{
		OnStreamDisconnect: func(exec *AgentExecution, disconnectErr error, generation uint64) {
			close(callbackReached)
			<-releaseCallback
			mgr.handleStreamDisconnect(exec, disconnectErr, generation)
			close(callbackDone)
		},
	}, nil, nil)
	go streamManager.handleUpdatesDisconnect(execution, errors.New("old stream lost"))

	<-callbackReached
	signal := <-execution.promptDoneCh
	if signal.PromptGeneration != originalGeneration {
		t.Fatalf("disconnect signal generation = %d, want %d", signal.PromptGeneration, originalGeneration)
	}

	replacementGeneration, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin replacement prompt: %v", err)
	}
	if replacementGeneration == originalGeneration {
		t.Fatal("replacement prompt did not advance generation")
	}
	resetStreamingStateWithHistory(execution, history, mgr.logger)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "replacement text"})

	close(releaseCallback)
	<-callbackDone

	if execution.Status != v1.AgentStatusRunning {
		t.Fatalf("replacement status = %s, want %s", execution.Status, v1.AgentStatusRunning)
	}
	execution.messageMu.Lock()
	assistantHistory := execution.assistantHistoryBuffer.String()
	messageBuffer := execution.messageBuffer.String()
	execution.messageMu.Unlock()
	if assistantHistory != "replacement text" || messageBuffer != "replacement text" {
		t.Fatalf("replacement buffers changed: history=%q message=%q", assistantHistory, messageBuffer)
	}

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "old partial" {
		t.Fatalf("history = %+v, want only the original partial response", entries)
	}
	for _, published := range eventBus.PublishedEvents {
		if published.Subject == events.AgentctlError {
			t.Fatal("superseded disconnect published AgentctlError")
		}
	}
}

func TestResetStreamingStateWithHistory_HasNoDrainResetGap(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	execution.assistantHistoryBuffer.WriteString("old partial")
	execution.messageBuffer.WriteString("old partial")

	// Hold persistence so the test can force a chunk to arrive after history
	// detaches. With split drain/reset locks, that chunk was accepted in the
	// gap and then erased by reset. The atomic helper resets before blocking
	// on history I/O, so the late chunk belongs to the post-reset buffer.
	history.mu.Lock()
	historyLocked := true
	defer func() {
		if historyLocked {
			history.mu.Unlock()
		}
	}()
	resetDone := make(chan struct{})
	go func() {
		resetStreamingStateWithHistory(execution, history, mgr.logger)
		close(resetDone)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		execution.messageMu.Lock()
		detached := execution.assistantHistoryBuffer.Len() == 0
		execution.messageMu.Unlock()
		if detached {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("assistant history did not detach")
		}
		runtime.Gosched()
	}
	mgr.handleMessageChunkEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "late chunk"})

	history.mu.Unlock()
	historyLocked = false
	<-resetDone

	execution.messageMu.Lock()
	assistantHistory := execution.assistantHistoryBuffer.String()
	messageBuffer := execution.messageBuffer.String()
	execution.messageMu.Unlock()
	if assistantHistory != "late chunk" || messageBuffer != "late chunk" {
		t.Fatalf("late chunk lost across reset: history=%q message=%q", assistantHistory, messageBuffer)
	}
	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "old partial" {
		t.Fatalf("detached history = %+v, want only old partial", entries)
	}
}

func TestStreamDisconnect_DiscardsHistoryWhenRecordingUnavailable(t *testing.T) {
	tests := []struct {
		name           string
		historyManager bool
		historyEnabled bool
		sessionID      string
	}{
		{name: "history manager unavailable", historyEnabled: true, sessionID: "session-1"},
		{name: "history disabled", historyManager: true, sessionID: "session-1"},
		{name: "session unavailable", historyManager: true, historyEnabled: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mgr, _ := createTestManagerWithTracking()
			mgr.historyManager = nil
			if test.historyManager {
				history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
				if err != nil {
					t.Fatalf("create history manager: %v", err)
				}
				mgr.historyManager = history
			}
			execution := createTestExecution("exec-1", "task-1", test.sessionID)
			execution.historyEnabled = test.historyEnabled
			execution.assistantHistoryBuffer.WriteString("discard me")
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("add execution: %v", err)
			}

			mgr.handleStreamDisconnect(execution, errors.New("connection lost"), 0)

			execution.messageMu.Lock()
			buffered := execution.assistantHistoryBuffer.Len()
			execution.messageMu.Unlock()
			if buffered != 0 {
				t.Fatalf("assistant history buffer length = %d, want discarded", buffered)
			}
		})
	}
}

func TestHandleAgentEvent_ProtocolAssistantAndToolHistoryFollowWireOrder(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: "message_chunk", Text: "before tool", ProtocolMessageID: "message-a"},
		{Type: "tool_call", ToolCallID: "tool-1", ToolName: "read_file", ToolStatus: "started"},
		{Type: "message_chunk", Text: "during tool", ProtocolMessageID: "message-a"},
		{Type: "tool_update", ToolCallID: "tool-1", ToolName: "read_file", ToolStatus: toolStatusComplete},
		{Type: "message_chunk", Text: "after result", ProtocolMessageID: "message-a"},
		{Type: "complete"},
	} {
		mgr.handleAgentEvent(execution, event)
	}

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("history entry count = %d, want 5: %+v", len(entries), entries)
	}
	wantTypes := []string{"agent_message", historyEntryTypeToolCall, "agent_message", "tool_result", "agent_message"}
	wantContent := []string{"before tool", "", "during tool", "", "after result"}
	for i := range wantTypes {
		if entries[i].Type != wantTypes[i] || entries[i].Content != wantContent[i] {
			t.Fatalf("entry %d = type:%q content:%q, want type:%q content:%q",
				i, entries[i].Type, entries[i].Content, wantTypes[i], wantContent[i])
		}
	}
}

func TestHandleAgentEvent_MixedAssistantHistoryPreservesAllTextAroundTool(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: "message_chunk", Text: "legacy before "},
		{Type: "message_chunk", Text: "protocol before", ProtocolMessageID: "message-a"},
		{Type: "tool_call", ToolCallID: "tool-1", ToolName: "read_file"},
		{Type: "message_chunk", Text: "legacy after "},
		{Type: "message_chunk", Text: "protocol after", ProtocolMessageID: "message-a"},
		{Type: "complete"},
	} {
		mgr.handleAgentEvent(execution, event)
	}

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("history entry count = %d, want 3: %+v", len(entries), entries)
	}
	if entries[0].Type != "agent_message" || entries[0].Content != "legacy before protocol before" {
		t.Fatalf("first history segment = %+v, want complete mixed pre-tool text", entries[0])
	}
	if entries[1].Type != historyEntryTypeToolCall {
		t.Fatalf("second history entry type = %q, want %q", entries[1].Type, historyEntryTypeToolCall)
	}
	if entries[2].Type != "agent_message" || entries[2].Content != "legacy after protocol after" {
		t.Fatalf("final history segment = %+v, want complete mixed post-tool text", entries[2])
	}

	messageEvents := streamEventsOfType(eventBus, "message_streaming")
	if len(messageEvents) != 4 {
		t.Fatalf("visible message event count = %d, want 4", len(messageEvents))
	}
	wantVisible := []string{"legacy before", "protocol before", "legacy after", "protocol after"}
	for i, want := range wantVisible {
		if messageEvents[i].Data.Text != want {
			t.Fatalf("visible event %d text = %q, want %q", i, messageEvents[i].Data.Text, want)
		}
	}
}

func TestHandleAgentEvent_HistoryDisabledDoesNotPersistOrLeakAssistantText(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: "message_chunk", Text: "legacy"},
		{Type: "message_chunk", Text: " protocol", ProtocolMessageID: "message-a"},
		{Type: "tool_call", ToolCallID: "tool-1", ToolName: "read_file"},
		{Type: "message_chunk", Text: "after", ProtocolMessageID: "message-a"},
		{Type: "complete"},
	} {
		mgr.handleAgentEvent(execution, event)
	}

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("history-disabled execution persisted entries: %+v", entries)
	}
	execution.messageMu.Lock()
	historyBufferLen := execution.assistantHistoryBuffer.Len()
	execution.messageMu.Unlock()
	if historyBufferLen != 0 {
		t.Fatalf("assistant history accumulator length = %d after completion, want 0", historyBufferLen)
	}
}

func TestHandleAgentEvent_EmptyHistoryBoundariesDoNotDuplicateAssistant(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	history, err := NewSessionHistoryManager(t.TempDir(), "", newTestLogger())
	if err != nil {
		t.Fatalf("create history manager: %v", err)
	}
	mgr.historyManager = history
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.historyEnabled = true
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	for _, event := range []agentctl.AgentEvent{
		{Type: "message_chunk", Text: "only once", ProtocolMessageID: "message-a"},
		{Type: "tool_call", ToolCallID: "tool-1", ToolName: "read_file"},
		{Type: "tool_update", ToolCallID: "tool-1", ToolName: "read_file", ToolStatus: toolStatusComplete},
		{Type: "complete"},
		{Type: "complete"},
	} {
		mgr.handleAgentEvent(execution, event)
	}

	entries, err := history.ReadHistory(execution.SessionID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("history entry count = %d, want assistant/tool/result only: %+v", len(entries), entries)
	}
	if entries[0].Type != "agent_message" || entries[0].Content != "only once" {
		t.Fatalf("assistant history entry = %+v, want one exact segment", entries[0])
	}
	if entries[1].Type != historyEntryTypeToolCall || entries[2].Type != "tool_result" {
		t.Fatalf("history boundary entries = (%q, %q), want tool_call/tool_result",
			entries[1].Type, entries[2].Type)
	}
}

func streamEventsOfType(eventBus *MockEventBusWithTracking, eventType string) []AgentStreamEventPayload {
	var matching []AgentStreamEventPayload
	for _, event := range eventBus.getStreamEvents() {
		if event.Data != nil && event.Data.Type == eventType {
			matching = append(matching, event)
		}
	}
	return matching
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

// TestHandleAgentEvent_SubagentToolCallDoesNotSplitStreaming is the regression test
// for streaming messages being shattered into multiple DB rows: a subagent (Task tool)
// streams its internal tool calls (tagged with ParentToolCallID) on the same session
// while the parent agent is still streaming text. Those tool calls must NOT flush the
// message buffer — otherwise every subagent tool call starts a new message row,
// splitting the parent's message mid-sentence and breaking markdown across rows.
func TestHandleAgentEvent_SubagentToolCallDoesNotSplitStreaming(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	// Parent agent starts streaming its message
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "Here's the CI picture\n",
	})

	execution.messageMu.Lock()
	msgIDBefore := execution.currentMessageID
	execution.messageMu.Unlock()
	if msgIDBefore == "" {
		t.Fatal("currentMessageID should be set after message_chunk")
	}

	// A subagent's internal tool call arrives mid-stream (ParentToolCallID set)
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:             "tool_call",
		ToolCallID:       "subagent-tool-1",
		ParentToolCallID: "parent-task-tool",
		ToolName:         "execute",
	})

	// The parent's streaming message must survive the subagent tool call
	execution.messageMu.Lock()
	msgIDAfter := execution.currentMessageID
	execution.messageMu.Unlock()
	if msgIDAfter != msgIDBefore {
		t.Errorf("subagent tool_call must not close the streaming message: message ID changed from %q to %q",
			msgIDBefore, msgIDAfter)
	}

	// More parent text — must APPEND to the same message, not create a new one
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk",
		Text: "on the new PR.\n",
	})

	var streamingEvents []AgentStreamEventPayload
	for _, e := range eventBus.getStreamEvents() {
		if e.Data != nil && e.Data.Type == "message_streaming" {
			streamingEvents = append(streamingEvents, e)
		}
	}
	if len(streamingEvents) < 2 {
		t.Fatalf("expected at least 2 message_streaming events, got %d", len(streamingEvents))
	}
	last := streamingEvents[len(streamingEvents)-1]
	if !last.Data.IsAppend {
		t.Error("text after a subagent tool_call should append to the existing message, got IsAppend=false")
	}
	if last.Data.MessageID != msgIDBefore {
		t.Errorf("text after a subagent tool_call should keep message ID %q, got %q",
			msgIDBefore, last.Data.MessageID)
	}

	// A top-level tool call (no ParentToolCallID) must still flush as before
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       "tool_call",
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})

	execution.messageMu.Lock()
	msgIDAfterTopLevel := execution.currentMessageID
	execution.messageMu.Unlock()
	if msgIDAfterTopLevel != "" {
		t.Errorf("currentMessageID should be cleared after a top-level tool_call, got %q", msgIDAfterTopLevel)
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

func TestHandleAgentEvent_DelayedCompleteCannotFinishReplacementPrompt(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	if _, err := mgr.executionStore.BeginPrompt(execution.ID); err != nil {
		t.Fatalf("begin original prompt: %v", err)
	}
	if _, err := mgr.executionStore.BeginPrompt(execution.ID); err != nil {
		t.Fatalf("begin replacement prompt: %v", err)
	}

	execution.messageMu.Lock()
	execution.messageBuffer.WriteString("replacement output")
	execution.currentMessageID = "replacement-message"
	execution.messageMu.Unlock()

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:             "complete",
		SessionID:        execution.SessionID,
		PromptGeneration: 1,
	})

	if execution.Status != v1.AgentStatusRunning {
		t.Fatalf("replacement status = %s, want %s", execution.Status, v1.AgentStatusRunning)
	}
	select {
	case signal := <-execution.promptDoneCh:
		t.Fatalf("delayed completion signaled replacement prompt: %+v", signal)
	default:
	}
	execution.messageMu.Lock()
	buffer := execution.messageBuffer.String()
	messageID := execution.currentMessageID
	execution.messageMu.Unlock()
	if buffer != "replacement output" || messageID != "replacement-message" {
		t.Fatalf("replacement stream state changed: buffer=%q message_id=%q", buffer, messageID)
	}
	for _, published := range eventBus.PublishedEvents {
		if published.Subject == events.AgentReady {
			t.Fatal("delayed completion published AgentReady for replacement prompt")
		}
	}
}

func TestHandleAgentEvent_ErrorClaimCannotRaceReplacementPrompt(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	if _, err := mgr.executionStore.BeginPrompt(execution.ID); err != nil {
		t.Fatalf("begin original prompt: %v", err)
	}

	errorReached := make(chan struct{})
	releaseError := make(chan struct{})
	var blockOnce sync.Once
	zapLogger := zap.NewExample().WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message != "agent error" && entry.Message != "agent turn complete" {
			return nil
		}
		blockOnce.Do(func() {
			close(errorReached)
			<-releaseError
		})
		return nil
	}))
	mgr.logger, _ = logger.NewFromZap(zapLogger)

	errorHandled := make(chan struct{})
	go func() {
		defer close(errorHandled)
		mgr.handleAgentEvent(execution, agentctl.AgentEvent{
			Type:             "error",
			Error:            "original prompt failed",
			SessionID:        execution.SessionID,
			PromptGeneration: 1,
		})
	}()
	<-errorReached

	errorOwnsLifecycle := !execution.promptLifecycleMu.TryLock()
	if !errorOwnsLifecycle {
		execution.promptLifecycleMu.Unlock()
	}

	replacementStarted := make(chan error, 1)
	go func() {
		_, err := mgr.executionStore.BeginPrompt(execution.ID)
		replacementStarted <- err
	}()
	writeReplacementOutput := func() {
		execution.messageMu.Lock()
		execution.messageBuffer.WriteString("replacement output")
		execution.currentMessageID = "replacement-message"
		execution.messageMu.Unlock()
	}
	if !errorOwnsLifecycle {
		if err := <-replacementStarted; err != nil {
			t.Fatalf("begin replacement prompt: %v", err)
		}
		writeReplacementOutput()
	}

	close(releaseError)
	if errorOwnsLifecycle {
		if err := <-replacementStarted; err != nil {
			t.Fatalf("begin replacement prompt: %v", err)
		}
		writeReplacementOutput()
	}
	<-errorHandled

	if !errorOwnsLifecycle {
		t.Error("error event released prompt lifecycle ownership before mutating execution state")
	}
	if execution.Status != v1.AgentStatusRunning {
		t.Fatalf("replacement status = %s, want %s", execution.Status, v1.AgentStatusRunning)
	}
	execution.messageMu.Lock()
	buffer := execution.messageBuffer.String()
	messageID := execution.currentMessageID
	execution.messageMu.Unlock()
	if buffer != "replacement output" || messageID != "replacement-message" {
		t.Fatalf("replacement stream state changed: buffer=%q message_id=%q", buffer, messageID)
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

// TestExtractErrorMessage verifies the priority chain: Error > Text > default.
func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name  string
		event *agentctl.AgentEvent
		want  string
	}{
		{
			name:  "Error field takes priority",
			event: &agentctl.AgentEvent{Error: "explicit error", Text: "text fallback"},
			want:  "explicit error",
		},
		{
			name:  "Text field used when Error is empty",
			event: &agentctl.AgentEvent{Error: "", Text: "text fallback"},
			want:  "text fallback",
		},
		{
			name:  "default when both empty",
			event: &agentctl.AgentEvent{Error: "", Text: ""},
			want:  "agent error completion",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrorMessage(tt.event)
			if got != tt.want {
				t.Errorf("extractErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHandleAgentEvent_ErrorCompleteWithTextFallback verifies that when an error
// completion has no Error field but has Text, the Text is used as the error message.
func TestHandleAgentEvent_ErrorCompleteWithTextFallback(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
		Text: "Operation timed out",
		Data: map[string]any{"is_error": true},
	})

	select {
	case signal := <-execution.promptDoneCh:
		if !signal.IsError {
			t.Error("expected error signal")
		}
		if signal.Error != "Operation timed out" {
			t.Errorf("expected error 'Operation timed out', got %q", signal.Error)
		}
	default:
		t.Error("expected signal on promptDoneCh, got none")
	}
}

// TestHandleAgentEvent_ErrorCompleteWithDefaultMessage verifies that when both
// Error and Text fields are empty, the default message is used.
func TestHandleAgentEvent_ErrorCompleteWithDefaultMessage(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "complete",
		Data: map[string]any{"is_error": true},
	})

	select {
	case signal := <-execution.promptDoneCh:
		if !signal.IsError {
			t.Error("expected error signal")
		}
		if signal.Error != "agent error completion" {
			t.Errorf("expected default error message, got %q", signal.Error)
		}
	default:
		t.Error("expected signal on promptDoneCh, got none")
	}
}

// TestHandleCompleteEventMarkState_ErrorDoesNotRemoveExecution verifies that on error
// completion, the execution is NOT removed from the store. The orchestrator's cleanup
// (StopExecution → StopAgentWithReason) handles full teardown including port release;
// premature removal would prevent that cleanup from finding the execution.
func TestHandleCompleteEventMarkState_ErrorDoesNotRemoveExecution(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	errorEvent := &agentctl.AgentEvent{
		Type:  "complete",
		Error: "agent crashed",
		Data:  map[string]interface{}{"is_error": true},
	}

	mgr.handleCompleteEventMarkState(execution, errorEvent, true)

	// Execution must still be in the store so the orchestrator can clean it up
	if _, found := mgr.executionStore.Get("exec-1"); !found {
		t.Error("execution was removed from store on error completion; " +
			"it should remain so the orchestrator can call StopExecution to release resources")
	}
}

// TestHandleCompleteEventMarkState_SuccessKeepsExecution verifies that on normal
// completion, the execution remains in the store (marked ready, not removed).
func TestHandleCompleteEventMarkState_SuccessKeepsExecution(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	mgr.executionStore.Add(execution)

	successEvent := &agentctl.AgentEvent{
		Type: "complete",
	}

	mgr.handleCompleteEventMarkState(execution, successEvent, false)

	got, found := mgr.executionStore.Get("exec-1")
	if !found {
		t.Error("execution was removed from store on successful completion; it should remain")
	}
	if found && got.Status != v1.AgentStatusReady {
		t.Errorf("expected status %q after success, got %q", v1.AgentStatusReady, got.Status)
	}
}
