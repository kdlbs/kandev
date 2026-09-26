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
