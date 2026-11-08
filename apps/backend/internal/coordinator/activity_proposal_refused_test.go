package coordinator

import (
	"context"
	"testing"
)

// A refusal that carries a proposal id counts in ProposalRefused and never in
// Refused, which keeps summing the tool-call refusals of rows without one.
func TestActivitySummarySeparatesProposalRefusals(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	svc := newPhase2Service(t, store, true)

	toolRefusal := validRow(c.ID)
	toolRefusal.ID, toolRefusal.Outcome, toolRefusal.ActionClass, toolRefusal.RefusalCount = "r-tool", ActivityRefused, ActionAnswer, 4
	if err := store.InsertActivity(ctx, store.db, toolRefusal); err != nil {
		t.Fatal(err)
	}
	pid := "prop-1"
	proposalRefusal := validRow(c.ID)
	proposalRefusal.ID, proposalRefusal.Outcome, proposalRefusal.ActionClass, proposalRefusal.RefusalCount = "r-prop", ActivityRefused, ActionAnswer, 1
	proposalRefusal.ProposalID = &pid
	if err := store.InsertActivity(ctx, store.db, proposalRefusal); err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ActivitySummary(ctx, c.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := sum.Classes[ActionAnswer]
	if got.Refused != 4 || got.ProposalRefused != 1 {
		t.Fatalf("answer counts = %+v, want Refused 4 and ProposalRefused 1", got)
	}
	if _, ok := sum.Classes[ActionAnswer]; !ok || len(sum.Classes) != len(AllActions) {
		t.Fatalf("classes = %v", sum.Classes)
	}
}
