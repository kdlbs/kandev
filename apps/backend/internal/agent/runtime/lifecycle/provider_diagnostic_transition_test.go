package lifecycle

import (
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity covers
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20: candidate markers stay on
// original observations and do not split visible assistant records.
func TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity(t *testing.T) {
	t.Run("legacy chunks keep one record", func(t *testing.T) {
		mgr, eventBus, execution := diagnosticTransitionFixture(t)
		chunks := []agentctl.AgentEvent{
			{Type: "message_chunk", Text: "934 `dial tcp ", AttemptID: "attempt-1"},
			{Type: "message_chunk", Text: "<ip>:6379: ", AttemptID: "attempt-1"},
			{Type: "message_chunk", Text: "i/o timeout` errors, ", ProviderDiagnosticCandidate: true, AttemptID: "attempt-1"},
			{Type: "message_chunk", Text: "meaning the TCP connect failed.", AttemptID: "attempt-1"},
		}
		for _, chunk := range chunks {
			mgr.handleAgentEvent(execution, chunk)
		}
		mgr.flushMessageBuffer(execution, 0, "attempt-1")
		mgr.flushStreamCoalescer(execution)

		assertOneAssistantRecord(t, streamEventsOfType(eventBus, "message_streaming"),
			"934 `dial tcp <ip>:6379: i/o timeout` errors, meaning the TCP connect failed.")
	})

	t.Run("protocol ID stays stable across markers", func(t *testing.T) {
		mgr, eventBus, execution := diagnosticTransitionFixture(t)
		for _, chunk := range []agentctl.AgentEvent{
			{Type: "message_chunk", Text: "before `", ProtocolMessageID: "protocol-1"},
			{Type: "message_chunk", Text: "i/o timeout", ProtocolMessageID: "protocol-1", ProviderDiagnosticCandidate: true},
			{Type: "message_chunk", Text: "` after", ProtocolMessageID: "protocol-1"},
		} {
			mgr.handleAgentEvent(execution, chunk)
		}
		mgr.flushStreamCoalescer(execution)

		assertOneAssistantRecord(t, streamEventsOfType(eventBus, "message_streaming"), "before `i/o timeout` after")
	})

	t.Run("newline flush and tool call retain real boundaries", func(t *testing.T) {
		mgr, eventBus, execution := diagnosticTransitionFixture(t)
		for _, chunk := range []agentctl.AgentEvent{
			{Type: "message_chunk", Text: "934 `dial tcp ", AttemptID: "attempt-2"},
			{Type: "message_chunk", Text: "<ip>:6379: ", AttemptID: "attempt-2"},
			{Type: "message_chunk", Text: "i/o timeout` errors,\n", ProviderDiagnosticCandidate: true, AttemptID: "attempt-2"},
			{Type: "message_chunk", Text: "meaning the TCP connect failed.\n", AttemptID: "attempt-2"},
		} {
			mgr.handleAgentEvent(execution, chunk)
		}
		mgr.handleAgentEvent(execution, agentctl.AgentEvent{
			Type:       "tool_call",
			ToolCallID: "tool-1",
			ToolName:   "read_file",
		})
		mgr.handleAgentEvent(execution, agentctl.AgentEvent{
			Type: "message_chunk",
			Text: "output after tool",
		})
		mgr.flushMessageBuffer(execution, 0, "")
		mgr.flushStreamCoalescer(execution)

		messageEvents := streamEventsOfType(eventBus, "message_streaming")
		if len(messageEvents) != 3 {
			t.Fatalf("message event count = %d, want two chunks before and one after the tool: %+v", len(messageEvents), messageEvents)
		}
		firstID := messageEvents[0].Data.MessageID
		if firstID == "" || messageEvents[1].Data.MessageID != firstID || messageEvents[2].Data.MessageID == firstID {
			t.Fatalf("message IDs = (%q, %q, %q), want stable pre-tool identity then a new post-tool identity", firstID, messageEvents[1].Data.MessageID, messageEvents[2].Data.MessageID)
		}
		if messageEvents[0].Data.IsAppend || !messageEvents[1].Data.IsAppend || messageEvents[2].Data.IsAppend {
			t.Fatalf("append flags = (%v, %v, %v), want (false, true, false)", messageEvents[0].Data.IsAppend, messageEvents[1].Data.IsAppend, messageEvents[2].Data.IsAppend)
		}
		if got := messageEvents[0].Data.Text + messageEvents[1].Data.Text; got != "934 `dial tcp <ip>:6379: i/o timeout` errors,\nmeaning the TCP connect failed.\n" {
			t.Fatalf("pre-tool content = %q", got)
		}
		if messageEvents[2].Data.Text != "output after tool" {
			t.Fatalf("post-tool content = %q", messageEvents[2].Data.Text)
		}
	})
}

func diagnosticTransitionFixture(t *testing.T) (*Manager, *MockEventBusWithTracking, *AgentExecution) {
	t.Helper()
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	return mgr, eventBus, execution
}

func assertOneAssistantRecord(t *testing.T, events []AgentStreamEventPayload, want string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("no message_streaming events")
	}
	messageID := events[0].Data.MessageID
	var content strings.Builder
	for index, event := range events {
		if event.Data == nil || event.Data.Type != "message_streaming" || event.Data.ProviderDiagnosticCandidate {
			t.Fatalf("visible event %d retained evidence-only state: %+v", index, event.Data)
		}
		if event.Data.MessageID != messageID || messageID == "" {
			t.Fatalf("visible event %d message ID = %q, want stable ID %q", index, event.Data.MessageID, messageID)
		}
		if event.Data.IsAppend != (index != 0) {
			t.Fatalf("visible event %d append = %v, want %v", index, event.Data.IsAppend, index != 0)
		}
		content.WriteString(event.Data.Text)
	}
	if content.String() != want {
		t.Fatalf("visible content = %q, want %q", content.String(), want)
	}
}
