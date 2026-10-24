package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestImprovementHiddenWhenPhase3Off(t *testing.T) {
	f := newImprovementFixture(t)
	p := f.mustPropose(t)
	f.svc.phase3 = false
	ctx := context.Background()
	if _, err := f.svc.GetProposal(ctx, "ws-1", f.c.ID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get err = %v", err)
	}
	list, err := f.svc.ListProposals(ctx, "ws-1", f.c.ID, ListProposalsPending)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range list {
		if row.ID == p.ID {
			t.Fatalf("improvement listed while phase 3 is off")
		}
	}
	if _, err := f.approve(p, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("approve err = %v", err)
	}
	if _, err := f.svc.RejectProposal(ctx, "ws-1", f.c.ID, p.ID, RejectProposalRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reject err = %v", err)
	}
	if statusOf(t, f.store, p.ID) != string(ProposalStatusPending) || f.changeRows(t) != 0 {
		t.Fatalf("hidden proposal was decided")
	}
}

func TestImprovementStaleClaimSettlesWhenPhase3Off(t *testing.T) {
	f := newImprovementFixture(t)
	p := f.mustPropose(t)
	spec := ProposalSpec{}
	claimDirectly(t, f.store, p, "old-tok", spec, time.Now().Add(-10*time.Minute))
	f.svc.phase3 = false

	f.svc.StartupRecoveryPass(context.Background(), time.Now())

	if got := statusOf(t, f.store, p.ID); got != string(ProposalStatusFailed) {
		t.Fatalf("status = %q, want failed", got)
	}
	if f.changeRows(t) != 0 {
		t.Fatalf("change rows = %d, want 0", f.changeRows(t))
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("recovery changed the context")
	}
}

func TestImprovementReplyTextNamesTheImprovement(t *testing.T) {
	env := newReplyEnv(t)
	p := &Proposal{
		CoordinatorID: env.coordinator.ID, WorkspaceID: env.coordinator.WorkspaceID,
		Kind: ProposalKindImprovement, RawSpec: `{"title":"Shorter wake prompts"}`,
	}
	if err := env.svc.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	if _, err := env.reply(p, "Cite two runs"); err != nil {
		t.Fatal(err)
	}
	got := env.messenger.reqs[0].Content
	if !strings.Contains(got, `improvement proposal "Shorter wake prompts"`) || !strings.Contains(got, "Cite two runs") {
		t.Fatalf("content = %q", got)
	}
}

func TestImprovementChangesDeletedWithCoordinatorAndWorkspace(t *testing.T) {
	f := newImprovementFixture(t)
	f.approved(t)
	if err := f.store.DeleteCoordinator(context.Background(), "ws-1", f.c.ID); err != nil {
		t.Fatal(err)
	}
	if f.changeRows(t) != 0 {
		t.Fatalf("change rows after coordinator delete = %d", f.changeRows(t))
	}
	g := newImprovementFixture(t)
	g.approved(t)
	if err := g.store.DeleteWorkspaceState(context.Background(), "ws-1"); err != nil {
		t.Fatal(err)
	}
	if g.changeRows(t) != 0 {
		t.Fatalf("change rows after workspace delete = %d", g.changeRows(t))
	}
}
