package mcpcontract

import "testing"

// TestDecisionActions_ReservesApproveAndReject pins the two MCP action names
// approve and reject must never be reachable under
// (docs/specs/coordinator/system-design/proposals.md#security): no handler
// registers them, and the coordinator guard refuses them by name before
// either a coordinator or an unresolved principal could reach a dispatcher
// that would otherwise answer "unknown action" for the same reason.
func TestDecisionActions_ReservesApproveAndReject(t *testing.T) {
	if ActionApproveProposal != "coordinator.approve_proposal" {
		t.Errorf("ActionApproveProposal = %q, want %q", ActionApproveProposal, "coordinator.approve_proposal")
	}
	if ActionRejectProposal != "coordinator.reject_proposal" {
		t.Errorf("ActionRejectProposal = %q, want %q", ActionRejectProposal, "coordinator.reject_proposal")
	}
	for _, action := range []string{ActionApproveProposal, ActionRejectProposal} {
		if _, ok := DecisionActions[action]; !ok {
			t.Errorf("DecisionActions missing %q", action)
		}
	}
	if len(DecisionActions) != 2 {
		t.Errorf("len(DecisionActions) = %d, want 2", len(DecisionActions))
	}
}
