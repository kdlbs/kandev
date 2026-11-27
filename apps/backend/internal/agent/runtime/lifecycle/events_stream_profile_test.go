package lifecycle

import (
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func TestAgentStreamEventCarriesConcreteExecutionProfile(t *testing.T) {
	for _, identity := range []string{"", "dynamic-frontier", "office-cto"} {
		t.Run("identity="+identity, func(t *testing.T) {
			mgr, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("exec-1", "task-1", "session-1")
			execution.AgentProfileID = "candidate-a"
			execution.OfficeAgentProfileID = identity
			mgr.eventPublisher.PublishAgentStreamEvent(execution, agentctl.AgentEvent{
				Type: "error", Error: "provider unavailable", PromptGeneration: 1,
			})

			streamEvents := eventBus.getStreamEvents()
			if len(streamEvents) != 1 {
				t.Fatalf("published stream events = %d, want 1", len(streamEvents))
			}
			encoded, err := json.Marshal(streamEvents[0])
			if err != nil {
				t.Fatalf("marshal stream event: %v", err)
			}
			var fields map[string]interface{}
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("unmarshal stream event: %v", err)
			}
			if fields["execution_profile_id"] != "candidate-a" {
				t.Fatalf("execution profile = %v, want candidate-a", fields["execution_profile_id"])
			}
			wantIdentity := identity
			if wantIdentity == "" {
				wantIdentity = "candidate-a"
			}
			if fields["agent_profile_id"] != wantIdentity {
				t.Fatalf("agent profile = %v, want %s", fields["agent_profile_id"], wantIdentity)
			}
		})
	}
}
