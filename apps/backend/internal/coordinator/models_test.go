package coordinator

import "testing"

// TestActionProposeTask_Value pins the MCP action name the copilot's
// propose_task_kandev tool dispatches, per
// docs/specs/coordinator/system-design/proposals.md#propose. The dispatch
// site is owned by a later work package; this only declares the constant.
func TestActionProposeTask_Value(t *testing.T) {
	if ActionProposeTask != "coordinator.propose_task" {
		t.Errorf("ActionProposeTask = %q, want %q", ActionProposeTask, "coordinator.propose_task")
	}
}

// TestActionGetItem_Value pins the coordinator package's re-export of the
// get_coordinator_item_kandev tool's MCP action name, per
// docs/specs/coordinator/system-design/copilot-tools.md#item-read.
func TestActionGetItem_Value(t *testing.T) {
	if ActionGetItem != "coordinator.get_item" {
		t.Errorf("ActionGetItem = %q, want %q", ActionGetItem, "coordinator.get_item")
	}
}

// TestDecisionActions_Value pins the coordinator package's re-export of the
// reserved approve/reject MCP action names, per
// docs/specs/coordinator/system-design/proposals.md#security.
func TestDecisionActions_Value(t *testing.T) {
	if ActionApproveProposal != "coordinator.approve_proposal" {
		t.Errorf("ActionApproveProposal = %q, want %q", ActionApproveProposal, "coordinator.approve_proposal")
	}
	if ActionRejectProposal != "coordinator.reject_proposal" {
		t.Errorf("ActionRejectProposal = %q, want %q", ActionRejectProposal, "coordinator.reject_proposal")
	}
	if ActionReplyProposal != "coordinator.reply_proposal" || ActionDeliverReply != "coordinator.deliver_reply" {
		t.Errorf("reply action names = %q, %q", ActionReplyProposal, ActionDeliverReply)
	}
	for _, action := range []string{ActionApproveProposal, ActionRejectProposal, ActionReplyProposal, ActionDeliverReply} {
		if _, ok := DecisionActions[action]; !ok {
			t.Errorf("DecisionActions missing %q", action)
		}
	}
}

// TestProposalStatus_Values pins the proposal status enum used by the store,
// service and DTOs.
func TestProposalStatus_Values(t *testing.T) {
	cases := map[ProposalStatus]string{
		ProposalStatusPending:   "pending",
		ProposalStatusApproving: "approving",
		ProposalStatusApproved:  "approved",
		ProposalStatusRejected:  "rejected",
		ProposalStatusFailed:    "failed",
		ProposalStatusReturned:  "returned",
	}
	for status, want := range cases {
		if string(status) != want {
			t.Errorf("status = %q, want %q", status, want)
		}
	}
}
