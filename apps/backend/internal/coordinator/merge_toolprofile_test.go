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
