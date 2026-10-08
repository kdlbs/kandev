package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestBuildAgentStreamEventDataCarriesProviderOperationID(t *testing.T) {
	data := buildAgentStreamEventData(streams.AgentEvent{
		Type:        streams.EventTypeTurnStarted,
		OperationID: "provider-turn-1",
	})
	if data.OperationID != "provider-turn-1" {
		t.Fatalf("operation id = %q, want provider-turn-1", data.OperationID)
	}
}

func TestBuildAgentStreamEventDataCarriesRetainedFailureDisposition(t *testing.T) {
	data := buildAgentStreamEventData(streams.AgentEvent{
		Type:                     "complete",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
	})
	if data.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("prompt failure disposition = %q, want %q",
			data.PromptFailureDisposition, streams.PromptFailureDispositionRetainRuntime)
	}
}
