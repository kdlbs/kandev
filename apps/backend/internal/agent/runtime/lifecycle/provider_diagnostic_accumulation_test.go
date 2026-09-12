package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestHandleMessageChunkEvent_CarriesProviderDiagnosticCandidateToStream pins
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20's "orchestrator message path"
// hop. A "message_chunk" AgentEvent does not reach PublishAgentStreamEvent
// directly: handleAgentEvent routes it through handleMessageChunkEvent's
// accumulation/coalescing buffer first. The marker must still reach the
// "message_streaming" AgentStreamEventData the orchestrator consumes, or the
// orchestrator's diagnostic-vs-output classification (which reads this field,
// not the message text) never sees a true value in production.
func TestHandleMessageChunkEvent_CarriesProviderDiagnosticCandidateToStream(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                        "message_chunk",
		Text:                        "API Error: 500 Internal server error.",
		ProtocolMessageID:           "msg-1",
		ProviderDiagnosticCandidate: true,
	})
	mgr.flushStreamCoalescer(execution)

	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 1 {
		t.Fatalf("published stream events = %d, want 1: %+v", len(streamEvents), streamEvents)
	}
	if streamEvents[0].Data == nil || !streamEvents[0].Data.ProviderDiagnosticCandidate {
		t.Fatalf("message_streaming event did not carry the marker: %+v", streamEvents[0].Data)
	}
}
