package runtime

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestDecisionToolIsNotRuntimeCapability(t *testing.T) {
	caps := Capabilities{}
	if caps.Allows(AvailableActionRecordStepDecision) {
		t.Fatal("decision tool availability must not be treated as a runtime capability")
	}
	for _, key := range caps.AllowedKeys() {
		if key == AvailableActionRecordStepDecision {
			t.Fatalf("runtime capability list must not include the decision tool: %v", caps.AllowedKeys())
		}
	}
}

func TestFromAgentNeverGrantsRecordStepDecision(t *testing.T) {
	ceo := &models.AgentInstance{
		ID:          "agent-ceo",
		WorkspaceID: "ws-1",
		Role:        models.AgentRoleCEO,
	}
	caps := FromAgent(ceo)
	if caps.Allows(AvailableActionRecordStepDecision) {
		t.Fatal("Allows must not authorize the decision tool")
	}
}
