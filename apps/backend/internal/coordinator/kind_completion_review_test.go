package coordinator

import "testing"

func TestMoveApprovalDoesNotClaimToHaveCreatedTarget(t *testing.T) {
	f := newKindsFixture(t)
	p := f.insertMove(t)
	got, err := f.approve(p, nil)
	if err != nil || got == nil || got.Status != ProposalStatusApproved {
		t.Fatalf("approval = %+v, err %v", got, err)
	}
	if got.TaskID != nil || got.TargetTaskID == nil || *got.TargetTaskID != *p.TargetTaskID {
		t.Fatalf("move recorded as task creation: task=%v target=%v; want target %s", got.TaskID, got.TargetTaskID, *p.TargetTaskID)
	}
}
