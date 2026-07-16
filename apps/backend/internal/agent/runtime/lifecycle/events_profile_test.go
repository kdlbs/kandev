package lifecycle

import "testing"

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
