package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestHandleMessageChunkEvent_LegacyBufferSplitsOnDiagnosticChange pins the
// boundary flushMessageBufferOnDiagnosticChange enforces on the ID-less
// legacy message path: an unmarked chunk arriving while a marked segment is
// still buffered (or vice versa) flushes the buffered segment under its own
// marker before the new chunk starts accumulating, instead of merging into
// one published segment that could only carry one marker value.
func TestHandleMessageChunkEvent_LegacyBufferSplitsOnDiagnosticChange(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                        "message_chunk",
		Text:                        "API Error: 500 Internal server error ",
		ProviderDiagnosticCandidate: true,
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                        "message_chunk",
		Text:                        "and now some ordinary assistant text",
		ProviderDiagnosticCandidate: false,
	})
	mgr.flushMessageBuffer(execution)
	mgr.flushStreamCoalescer(execution)

	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 2 {
		t.Fatalf("published stream events = %d, want 2 (no cross-marker merge): %+v", len(streamEvents), streamEvents)
	}
	wantMarkers := []bool{true, false}
	wantTexts := []string{"API Error: 500 Internal server error ", "and now some ordinary assistant text"}
	for i, ev := range streamEvents {
		if ev.Data == nil {
			t.Fatalf("stream event %d carries nil data", i)
		}
		if ev.Data.ProviderDiagnosticCandidate != wantMarkers[i] {
			t.Fatalf("stream event %d marker = %v, want %v: %+v", i, ev.Data.ProviderDiagnosticCandidate, wantMarkers[i], ev.Data)
		}
		if ev.Data.Text != wantTexts[i] {
			t.Fatalf("stream event %d text = %q, want %q (a merge would concatenate segments and lose this boundary)", i, ev.Data.Text, wantTexts[i])
		}
	}
}

// TestHandleMessageChunkEvent_ProtocolIDStreamPreservesDiagnosticBoundary pins
// the same marker-awareness guarantee on the protocol-ID-bearing path.
// publishProtocolMessage assigns one stable messageID per ProtocolMessageID
// (manager_streaming.go:protocolRecordID), so a single logical ACP message
// can legitimately span both diagnostic-classifying and non-classifying
// chunks: adapter_updates.go's convertMessageChunkWithProtocolID classifies
// each chunk's own text, never the accumulated buffer. Without
// streamCoalescer.add's own lastDiagnostic check, an append chunk whose
// diagnostic value differs from the segment currently pending in the
// coalescer would silently merge into it, erasing the marker for whichever
// half of the merged text disagreed with it.
//
// This test forces exactly that pending-merge opportunity: chunk 1 publishes
// immediately (first chunk of a new message), chunk 2 is a non-diagnostic
// append that becomes the coalescer's pending segment, and chunk 3 is a
// diagnostic append that must NOT merge into that pending segment.
func TestHandleMessageChunkEvent_ProtocolIDStreamPreservesDiagnosticBoundary(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                        "message_chunk",
		Text:                        "Header: ",
		ProtocolMessageID:           "msg-1",
		ProviderDiagnosticCandidate: false,
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                        "message_chunk",
		Text:                        "ordinary continued ",
		ProtocolMessageID:           "msg-1",
		ProviderDiagnosticCandidate: false,
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:                        "message_chunk",
		Text:                        "API Error: 500 Internal Server Error",
		ProtocolMessageID:           "msg-1",
		ProviderDiagnosticCandidate: true,
	})
	mgr.flushStreamCoalescer(execution)

	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 3 {
		t.Fatalf("published stream events = %d, want 3 (no cross-marker merge): %+v", len(streamEvents), streamEvents)
	}
	wantMarkers := []bool{false, false, true}
	wantTexts := []string{"Header: ", "ordinary continued ", "API Error: 500 Internal Server Error"}
	for i, ev := range streamEvents {
		if ev.Data == nil {
			t.Fatalf("stream event %d carries nil data", i)
		}
		if ev.Data.ProviderDiagnosticCandidate != wantMarkers[i] {
			t.Fatalf("stream event %d marker = %v, want %v: %+v", i, ev.Data.ProviderDiagnosticCandidate, wantMarkers[i], ev.Data)
		}
		if ev.Data.Text != wantTexts[i] {
			t.Fatalf("stream event %d text = %q, want %q (a merge would concatenate segments and lose this boundary)", i, ev.Data.Text, wantTexts[i])
		}
	}
}
