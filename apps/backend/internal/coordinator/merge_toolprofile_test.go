package coordinator

import (
	"slices"
	"testing"
)

func TestMergeToolListedOnlyWhileFlagOnAndSomeRepositoryNotDenied(t *testing.T) {
	cases := []struct {
		name   string
		flag   bool
		policy RepoPolicy
		want   bool
	}{
		{"requires approval", true, RepoRequiresApproval, true},
		{"automatic", true, RepoAutomatic, true},
		{"denied", true, RepoDenied, false},
		{"flag off", false, RepoRequiresApproval, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newMergeExec(t)
			e.setPolicy(t, tc.policy, 0)
			e.svc.merge = tc.flag
			got := slices.Contains(e.svc.bindingToolNames(e.fix.c), mergeTool)
			if got != tc.want {
				t.Fatalf("listed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeToolMapsBackToItsAction(t *testing.T) {
	if name, ok := ToolForAction(ActionProposeMerge); !ok || name != mergeTool {
		t.Fatalf("ToolForAction = %q, %v", name, ok)
	}
	if got := ActionForTool(mergeTool); got != ActionUnknown {
		t.Fatalf("the merge tool exercises no policy action, got %q", got)
	}
}

func TestFlagOffHidesAndBlocks(t *testing.T) {
	e := newMergeExec(t)
	id := *e.propose(t, "h1").ProposalID
	e.svc.merge = false
	if slices.Contains(e.svc.bindingToolNames(e.fix.c), mergeTool) {
		t.Fatal("the merge tool is listed with the flag off")
	}
	if _, err := e.svc.ProposeMerge(e.ctx, e.fix.c.ID, []byte(`{"task_id":"tm","expected_head_sha":"h1"}`)); err == nil {
		t.Fatal("a merge proposal was created with the flag off")
	}
	if _, err := e.svc.CheckMergeNow(e.ctx, e.fix.c.WorkspaceID, e.fix.c.ID, id); err == nil {
		t.Fatal("check now ran with the flag off")
	}
	if _, d := e.svc.OpenMergeDetail(e.ctx, &Proposal{ID: id, Kind: ProposalKindMerge}); d != nil {
		t.Fatal("detail shown with the flag off")
	}
	if st, _ := e.status(t, id); st != "pending" {
		t.Fatalf("data not preserved: status = %s", st)
	}
}
