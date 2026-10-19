package replay

import "testing"

func TestRunPassesRequestCoordinatorToProposalAndTitleReads(t *testing.T) {
	w := newWorld()
	w.addCase("turn", caseProp{"proposal", DecisionApproved})
	r := req()
	r.CoordinatorID = "owned-coordinator"
	mustRun(t, w, r)
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.caseScopes) < 2 {
		t.Fatalf("proposal and title reads did not run: %v", w.caseScopes)
	}
	for _, scope := range w.caseScopes {
		if scope != r.CoordinatorID {
			t.Fatalf("case read adopted a different coordinator: %q", scope)
		}
	}
}
