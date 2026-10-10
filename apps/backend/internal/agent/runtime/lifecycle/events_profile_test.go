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
	payload := newAgentEventPayload(&AgentExecution{
		ID: "exec-1", WorkspaceID: "ws-1", RunID: "run-1", RunSessionID: "run-session-1", RunAttempt: 2,
	})
	if payload.RunID != "run-1" {
		t.Fatalf("run ID = %q, want run-1", payload.RunID)
	}
	if payload.OwnerKind != ExecutionOwnerRun || payload.WorkspaceID != "ws-1" ||
		payload.RunSessionID != "run-session-1" || payload.RunAttempt != 2 {
		t.Fatalf("run owner = %#v, want exact run identity", payload)
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

func TestAgentEventPayloadCarriesHostSettingsPolicy(t *testing.T) {
	providerRestored := newAgentEventPayload(&AgentExecution{
		ID:                              "exec-provider-restored",
		SessionSettingsProjectionPolicy: SessionSettingsPolicyProviderRestored,
	})
	if providerRestored.SessionSettingsPolicy != streams.SessionSettingsPolicyProviderRestored {
		t.Fatalf("provider-restored policy = %q, want provider_restored", providerRestored.SessionSettingsPolicy)
	}

	ordinary := newAgentEventPayload(&AgentExecution{ID: "exec-ordinary"})
	if ordinary.SessionSettingsPolicy != "" {
		t.Fatalf("ordinary policy = %q, want no recovery provenance", ordinary.SessionSettingsPolicy)
	}

	encoded, err := json.Marshal(providerRestored)
	if err != nil {
		t.Fatalf("marshal provider-restored lifecycle event: %v", err)
	}
	var watcherPayload map[string]interface{}
	if err := json.Unmarshal(encoded, &watcherPayload); err != nil {
		t.Fatalf("unmarshal provider-restored lifecycle event: %v", err)
	}
	if watcherPayload["session_settings_policy"] != string(streams.SessionSettingsPolicyProviderRestored) {
		t.Fatalf("wire policy = %#v, want provider_restored", watcherPayload["session_settings_policy"])
	}
}
