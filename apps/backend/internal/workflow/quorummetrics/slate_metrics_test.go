package quorummetrics

import (
	"expvar"
	"testing"
)

func TestRecordQuorumSlateEmpty(t *testing.T) {
	before := readCounter(t, workflowQuorumSlateEmpty, "reviewer")
	RecordQuorumSlateEmpty("reviewer")
	after := readCounter(t, workflowQuorumSlateEmpty, "reviewer")

	if after-before != 1 {
		t.Errorf("counter delta = %d, want 1", after-before)
	}
}

func TestRecordQuorumSlateEmpty_DistinctPerRole(t *testing.T) {
	beforeReviewer := readCounter(t, workflowQuorumSlateEmpty, "slate-empty-role-isolation-reviewer")
	beforeApprover := readCounter(t, workflowQuorumSlateEmpty, "slate-empty-role-isolation-approver")

	RecordQuorumSlateEmpty("slate-empty-role-isolation-reviewer")

	afterReviewer := readCounter(t, workflowQuorumSlateEmpty, "slate-empty-role-isolation-reviewer")
	afterApprover := readCounter(t, workflowQuorumSlateEmpty, "slate-empty-role-isolation-approver")

	if afterReviewer-beforeReviewer != 1 {
		t.Errorf("reviewer counter delta = %d, want 1", afterReviewer-beforeReviewer)
	}
	if afterApprover != beforeApprover {
		t.Errorf("approver counter changed: before=%d after=%d, want unchanged", beforeApprover, afterApprover)
	}
}

func TestWorkflowQuorumSlateEmptyPublishedAtKnownName(t *testing.T) {
	if expvar.Get("workflow_quorum_slate_empty_total") == nil {
		t.Error("expvar \"workflow_quorum_slate_empty_total\" not published — /debug/vars consumers will miss it")
	}
}
