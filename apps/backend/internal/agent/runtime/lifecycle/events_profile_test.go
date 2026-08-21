package lifecycle

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestAgentEventPayloadSeparatesOfficeAndExecutionProfiles(t *testing.T) {
	payload := newAgentEventPayload(&AgentExecution{
		ID: "exec-1", AgentProfileID: "claude-opus", OfficeAgentProfileID: "office-cto",
	})
	if payload.AgentProfileID != "office-cto" {
		t.Fatalf("agent profile = %q, want stable Office identity", payload.AgentProfileID)
	}
	if payload.ExecutionProfileID != "claude-opus" {
		t.Fatalf("execution profile = %q, want concrete CLI profile", payload.ExecutionProfileID)
	}
}

func TestAgentEventPayloadCarriesProviderErrorAndAgentID(t *testing.T) {
	occurred := time.Date(2026, 8, 2, 15, 15, 44, 0, time.UTC)
	payload := newAgentEventPayload(&AgentExecution{
		ID:      "exec-1",
		AgentID: "opencode-acp",
		ProviderError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceOpenCodeStderr,
			ModelID:    "kimi-k3",
			Message:    "5-hour usage limit reached",
			OccurredAt: occurred,
		},
	})
	if payload.AgentID != "opencode-acp" {
		t.Fatalf("agent ID = %q, want opencode-acp", payload.AgentID)
	}
	if payload.ProviderError == nil || payload.ProviderError.ModelID != "kimi-k3" {
		t.Fatalf("provider error = %+v", payload.ProviderError)
	}
}

func TestAgentEventPayloadCarriesRunID(t *testing.T) {
	payload := newAgentEventPayload(&AgentExecution{ID: "exec-1", RunID: "run-1"})
	if payload.RunID != "run-1" {
		t.Fatalf("run ID = %q, want run-1", payload.RunID)
	}
}

func TestAgentEventPayloadCarriesPromptTurnID(t *testing.T) {
	execution := &AgentExecution{ID: "exec-1", promptTurnID: "turn-1"}
	payload := newAgentEventPayload(execution)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var fields map[string]string
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if fields["turn_id"] != "turn-1" {
		t.Fatalf("turn_id = %q, want turn-1", fields["turn_id"])
	}
}
