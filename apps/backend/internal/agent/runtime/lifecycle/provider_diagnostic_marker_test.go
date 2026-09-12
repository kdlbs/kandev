package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestPublishAgentStreamEvent_CarriesProviderDiagnosticCandidate pins
// AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20: the marker decided once at the
// ACP conversion boundary must survive unchanged onto the
// AgentStreamEventData value published for the orchestrator. The two types
// are copied field-by-field rather than sharing a struct, so a future field
// addition here must not silently drop this one.
func TestPublishAgentStreamEvent_CarriesProviderDiagnosticCandidate(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
		Type:                        "tool_call",
		ToolCallID:                  "tool-1",
		ProviderDiagnosticCandidate: true,
	})

	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 1 {
		t.Fatalf("published stream events = %d, want 1", len(streamEvents))
	}
	if streamEvents[0].Data == nil || !streamEvents[0].Data.ProviderDiagnosticCandidate {
		t.Fatalf("published AgentStreamEventData did not carry the marker: %+v", streamEvents[0].Data)
	}
}
