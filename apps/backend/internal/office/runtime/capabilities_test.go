package runtime

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestCapabilityRecordStepDecisionRoundTripsThroughAllowsAndAllowedKeys(t *testing.T) {
	caps := Capabilities{CanRecordStepDecision: true}
	if !caps.Allows(CapabilityRecordStepDecision) {
		t.Fatal("expected Allows to report the granted decision capability")
	}
	keys := caps.AllowedKeys()
	found := false
	for _, k := range keys {
		if k == CapabilityRecordStepDecision {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected AllowedKeys to include %q, got %v", CapabilityRecordStepDecision, keys)
	}

	denied := Capabilities{CanRecordStepDecision: false}
	if denied.Allows(CapabilityRecordStepDecision) {
		t.Fatal("expected Allows to report false when the field is unset")
	}
}

func TestFromAgentNeverGrantsRecordStepDecision(t *testing.T) {
	ceo := &models.AgentInstance{
		ID:          "agent-ceo",
		WorkspaceID: "ws-1",
		Role:        models.AgentRoleCEO,
	}
	caps := FromAgent(ceo)
	if caps.CanRecordStepDecision {
		t.Fatal("FromAgent must never grant record_step_decision, even though the CEO role defaults to can_approve; the seat, not a permission, is the authority")
	}
	if caps.Allows(CapabilityRecordStepDecision) {
		t.Fatal("Allows should agree with the unset field")
	}
}
