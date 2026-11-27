package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
)

func managerCtx(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID})
}

func TestReplyRecordsDeciderAndReusesOneConversation(t *testing.T) {
	env := newReplyEnv(t)
	first, second := env.propose(t), env.propose(t)
	ctx := managerCtx("manager-a")
	for _, p := range []*Proposal{first, second} {
		if _, err := env.svc.ReplyToProposal(ctx, p.WorkspaceID, p.CoordinatorID, p.ID, ReplyRequest{Text: "narrower"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(env.messenger.reqs) != 2 {
		t.Fatalf("messages = %d", len(env.messenger.reqs))
	}
	a, b := env.messenger.reqs[0], env.messenger.reqs[1]
	if a.TaskSessionID == "" || a.TaskSessionID != b.TaskSessionID || a.TaskID != b.TaskID {
		t.Fatalf("replies did not share one opened conversation: %+v %+v", a, b)
	}
	if a.AuthorID != "manager-a" || a.AuthorType != "user" {
		t.Fatalf("author = %q/%q", a.AuthorType, a.AuthorID)
	}
	page, err := env.svc.ListActivity(context.Background(), first.WorkspaceID, first.CoordinatorID, ListActivityParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Rows {
		if row.Outcome == ActivityReturned && (row.ActorUserID == nil || *row.ActorUserID != "manager-a") {
			t.Fatalf("returned row actor = %v", row.ActorUserID)
		}
	}
}

func TestInReplyToRefusesNonCreateTaskAndForeignCoordinator(t *testing.T) {
	env := newReplyEnv(t)
	ctx := context.Background()
	c := env.coordinator

	move := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: ProposalKindMove, Spec: sampleSpec()}
	if err := env.svc.store.InsertProposal(ctx, move, true); err != nil {
		t.Fatal(err)
	}
	if _, err := env.reply(move, "no"); err != nil {
		t.Fatal(err)
	}
	assertFieldError(t, env.svc.checkInReplyTo(ctx, c, move.ID), "in_reply_to")

	other := *c
	other.ID, other.Name = "", "Other"
	if err := env.svc.store.CreateCoordinator(ctx, &other); err != nil {
		t.Fatal(err)
	}
	p := env.propose(t)
	if _, err := env.reply(p, "revise"); err != nil {
		t.Fatal(err)
	}
	assertFieldError(t, env.svc.checkInReplyTo(ctx, &other, p.ID), "in_reply_to")
}

func TestReturnedProposalFreesTheOpenCap(t *testing.T) {
	env := newReplyEnv(t)
	ctx := context.Background()
	var first *Proposal
	for i := 0; i < maxOpenProposals; i++ {
		p := env.propose(t)
		if first == nil {
			first = p
		}
	}
	extra := &Proposal{CoordinatorID: env.coordinator.ID, WorkspaceID: env.coordinator.WorkspaceID, Spec: sampleSpec()}
	if err := env.svc.store.InsertProposal(ctx, extra, true); err != ErrCoordinatorProposalCapReached {
		t.Fatalf("at the cap err = %v", err)
	}
	if _, err := env.reply(first, "revise"); err != nil {
		t.Fatal(err)
	}
	if err := env.svc.store.InsertProposal(ctx, extra, true); err != nil {
		t.Fatalf("a returned proposal still counts toward the cap: %v", err)
	}
}

func TestProposeTaskPersistsInReplyToAndLeavesReturnedRowUntouched(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.phase2, f.svc.phase3 = true, true
	ctx := context.Background()
	orig, _, err := f.svc.ProposeTask(ctx, f.coordinator.ID, f.baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.store.ReturnProposalTx(ctx, f.svc.store.db, orig.ID, "narrower", "m1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	req := f.baseRequest()
	req.InReplyTo = orig.ID
	revised, _, err := f.svc.ProposeTask(ctx, f.coordinator.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if revised.InReplyTo == nil || *revised.InReplyTo != orig.ID {
		t.Fatalf("in_reply_to = %v", revised.InReplyTo)
	}
	got, err := f.svc.store.GetProposal(ctx, f.workspaceID, f.coordinator.ID, orig.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ProposalStatusReturned || got.InReplyTo != nil {
		t.Fatalf("returned row changed: %+v", got)
	}
}
