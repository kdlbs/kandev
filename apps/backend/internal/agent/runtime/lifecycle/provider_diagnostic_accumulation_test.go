package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestHandleMessageChunkEvent_PublishesOriginalDiagnosticEvidence covers
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20 and .21: recovery sees each
// original assistant chunk while the transcript projection stays marker-free.
func TestHandleMessageChunkEvent_PublishesOriginalDiagnosticEvidence(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution.promptLifecycleMu.Lock()
	execution.promptGeneration = 7
	execution.promptLifecycleMu.Unlock()

	const text = "API Error: 500 Internal server error."
	chunks := []agentctl.AgentEvent{
		{
			Type:                        "message_chunk",
			Text:                        text,
			Role:                        "assistant",
			ProtocolMessageID:           "protocol-message-7",
			PromptGeneration:            7,
			ProviderDiagnosticCandidate: true,
		},
		{
			Type:              "message_chunk",
			Text:              " later prose",
			Role:              "assistant",
			ProtocolMessageID: "protocol-message-7",
		},
	}
	for _, chunk := range chunks {
		mgr.handleAgentEventWithAttempt(execution, chunk, "attempt-7")
	}
	mgr.flushMessageBuffer(execution, 7, "attempt-7")
	mgr.flushStreamCoalescer(execution)

	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 4 {
		t.Fatalf("evidence and visible events = %d, want two of each: %+v", len(streamEvents), streamEvents)
	}
	wantRawText := []string{text, " later prose"}
	for index, wantText := range wantRawText {
		payload := streamEvents[index*2]
		if data := payload.Data; data == nil || data.Type != "message_chunk" || data.Text != wantText ||
			data.Role != "assistant" || data.PromptGeneration != 7 || data.ProtocolMessageID != "protocol-message-7" ||
			data.MessageID != "" || payload.AttemptID != "attempt-7" {
			t.Fatalf("original chunk %d lost its evidence or correlation identity: payload=%+v data=%+v", index, payload, payload.Data)
		}
		if payload.Data.ProviderDiagnosticCandidate != (index == 0) {
			t.Fatalf("original chunk %d marker = %v, want %v", index, payload.Data.ProviderDiagnosticCandidate, index == 0)
		}
	}

	var visibleMessageID string
	for index := 1; index < len(streamEvents); index += 2 {
		data := streamEvents[index].Data
		if data == nil || data.Type != "message_streaming" || data.ProviderDiagnosticCandidate {
			t.Fatalf("visible transcript projection %d retained evidence state: %+v", index/2, data)
		}
		if visibleMessageID == "" {
			visibleMessageID = data.MessageID
		} else if data.MessageID != visibleMessageID {
			t.Fatalf("protocol chunks generated different transcript IDs: %+v", streamEvents)
		}
	}
}

func TestHandleReasoningEvent_PublishesOriginalTextAsEvidence(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	const text = "reasoning before a newline"
	mgr.handleAgentEventWithAttempt(execution, agentctl.AgentEvent{
		Type:              "reasoning",
		ReasoningText:     text,
		ProtocolMessageID: "thought-message-1",
		PromptGeneration:  8,
	}, "attempt-8")
	mgr.flushMessageBuffer(execution, 8, "attempt-8")
	mgr.flushStreamCoalescer(execution)

	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 2 || streamEvents[0].Data == nil || streamEvents[1].Data == nil {
		t.Fatalf("reasoning evidence and transcript events = %d, want one of each: %+v", len(streamEvents), streamEvents)
	}
	if data := streamEvents[0].Data; data.Type != "reasoning" || data.Text != text ||
		data.ProtocolMessageID != "thought-message-1" || data.MessageID != "" ||
		data.PromptGeneration != 8 || streamEvents[0].AttemptID != "attempt-8" {
		t.Fatalf("reasoning evidence lost its text or correlation identity: payload=%+v data=%+v", streamEvents[0], streamEvents[0].Data)
	}
	if data := streamEvents[1].Data; data.Type != thinkingStreamingEventType || data.MessageID == "" {
		t.Fatalf("visible reasoning projection = %+v, want one thinking_streaming event after evidence", data)
	}
}
