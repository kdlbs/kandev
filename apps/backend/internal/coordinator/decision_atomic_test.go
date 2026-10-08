package coordinator

import (
	"context"
	"testing"
	"time"
)

func claimedPhase2(t *testing.T, store *Store, c *Coordinator) (*Proposal, string) {
	t.Helper()
	p := insertKind(t, store, c, ProposalKindCreateTask)
	claimDirectly(t, store, p, "tok", sampleSpec(), time.Now())
	return p, "tok"
}

func dropActivity(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`DROP TABLE coordinator_activity`); err != nil {
		t.Fatal(err)
	}
}

// A failed activity write rolls the status change back for every decision.
func TestSettleDecision_RecordFailureRollsBackStatus(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		want ProposalStatus
		run  func(svc *Service, c *Coordinator, p *Proposal, tok string) (bool, error)
		pre  bool
	}{
		{"complete", ProposalStatusApproving, func(svc *Service, c *Coordinator, p *Proposal, tok string) (bool, error) {
			return svc.completeProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "task-x")
		}, true},
		{"fail", ProposalStatusApproving, func(svc *Service, c *Coordinator, p *Proposal, tok string) (bool, error) {
			return svc.failProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "boom")
		}, true},
		{"reject", ProposalStatusPending, func(svc *Service, _ *Coordinator, p *Proposal, _ string) (bool, error) {
			return svc.rejectProposalStore(ctx, p, "no", "u1")
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, c, svc := phase2Fixture(t, true)
			var p *Proposal
			tok := ""
			if tc.pre {
				p, tok = claimedPhase2(t, store, c)
			} else {
				p = insertKind(t, store, c, ProposalKindCreateTask)
			}
			dropActivity(t, store)
			matched, err := tc.run(svc, c, p, tok)
			if err == nil || matched {
				t.Fatalf("matched=%v err=%v, want error and not matched", matched, err)
			}
			if got := statusOf(t, store, p.ID); got != string(tc.want) {
				t.Fatalf("status = %q, want %q (rolled back)", got, tc.want)
			}
		})
	}
}

// A coordinator deleted mid-decision reads as not matched so the caller answers 404.
func TestSettleDecision_DeletedCoordinatorNotMatched(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"complete", "fail"} {
		t.Run(name, func(t *testing.T) {
			store, c, svc := phase2Fixture(t, true)
			p, tok := claimedPhase2(t, store, c)
			if err := store.DeleteCoordinator(ctx, c.WorkspaceID, c.ID); err != nil {
				t.Fatal(err)
			}
			var matched bool
			var err error
			if name == "complete" {
				matched, err = svc.completeProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "task-x")
			} else {
				matched, err = svc.failProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "boom")
			}
			if err != nil || matched {
				t.Fatalf("matched=%v err=%v, want false/nil", matched, err)
			}
			if _, err := svc.claimRaceResult(ctx, c.WorkspaceID, c.ID, p.ID); err == nil {
				t.Fatal("claimRaceResult after delete: want ErrNotFound")
			}
		})
	}
}
