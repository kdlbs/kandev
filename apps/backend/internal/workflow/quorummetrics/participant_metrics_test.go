package quorummetrics

import (
	"expvar"
	"testing"
)

func TestRecordParticipantAgentUnresolved(t *testing.T) {
	before := readCounter(t, workflowParticipantAgentUnresolved, "reviewer")
	RecordParticipantAgentUnresolved("reviewer")
	after := readCounter(t, workflowParticipantAgentUnresolved, "reviewer")

	if after-before != 1 {
		t.Errorf("counter delta = %d, want 1", after-before)
	}
}

func TestRecordParticipantAgentUnresolved_DistinctPerRole(t *testing.T) {
	beforeReviewer := readCounter(t, workflowParticipantAgentUnresolved, "approver-role-isolation-reviewer")
	beforeApprover := readCounter(t, workflowParticipantAgentUnresolved, "approver-role-isolation-approver")

	RecordParticipantAgentUnresolved("approver-role-isolation-reviewer")

	afterReviewer := readCounter(t, workflowParticipantAgentUnresolved, "approver-role-isolation-reviewer")
	afterApprover := readCounter(t, workflowParticipantAgentUnresolved, "approver-role-isolation-approver")

	if afterReviewer-beforeReviewer != 1 {
		t.Errorf("reviewer counter delta = %d, want 1", afterReviewer-beforeReviewer)
	}
	if afterApprover != beforeApprover {
		t.Errorf("approver counter changed: before=%d after=%d, want unchanged", beforeApprover, afterApprover)
	}
}

func TestWorkflowParticipantAgentUnresolvedPublishedAtKnownName(t *testing.T) {
	if expvar.Get("workflow_participant_agent_unresolved_total") == nil {
		t.Error("expvar \"workflow_participant_agent_unresolved_total\" not published — /debug/vars consumers will miss it")
	}
}
