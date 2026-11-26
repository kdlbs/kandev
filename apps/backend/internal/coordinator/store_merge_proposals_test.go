package coordinator

import (
	"context"
	"testing"
	"time"
)

func mergeRow(c *Coordinator, proposalID, key, head string) MergeProposal {
	return MergeProposal{
		ProposalID: proposalID, WorkspaceID: c.WorkspaceID, CoordinatorID: c.ID, RepositoryID: "r1",
		ChangeKey: key, HeadSHA: head, Method: "squash",
	}
}

func insertMergeProposalFor(t *testing.T, store *Store, c *Coordinator, key, head string) *Proposal {
	t.Helper()
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: ProposalKindMerge, RawSpec: `{}`, TargetTaskID: optString("t1")}
	err := store.InsertProposalWith(context.Background(), p, true, nil, func(ctx context.Context, tx coordinatorExec, p *Proposal) error {
		return store.insertMergeRowTx(ctx, tx, mergeRow(c, p.ID, key, head))
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUniqueIndexesIncludeWorkspace(t *testing.T) {
	store, c, _ := mergeEnv(t)
	insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	other := *c
	other.WorkspaceID = "other-ws"
	other.ID = "other-coordinator"
	mustExec(t, store, `INSERT INTO coordinators (id, workspace_id, name, agent_profile_id, executor_profile_id, created_at, updated_at) VALUES ('other-coordinator','other-ws','x','p','e',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	insertMergeProposalFor(t, store, &other, "github|github.com|r1|7", "aaa")
}

func TestOpenChangeKeyIsUniqueAcrossCoordinators(t *testing.T) {
	store, c, _ := mergeEnv(t)
	first := insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: ProposalKindMerge, RawSpec: `{}`, TargetTaskID: optString("t2")}
	err := store.InsertProposalWith(context.Background(), p, true, nil, func(ctx context.Context, tx coordinatorExec, p *Proposal) error {
		return store.insertMergeRowTx(ctx, tx, mergeRow(c, p.ID, "github|github.com|r1|7", "bbb"))
	})
	if err == nil {
		t.Fatal("a second open merge proposal for one change was inserted")
	}
	got, err := store.openMergeForChange(context.Background(), store.db, c.WorkspaceID, "github|github.com|r1|7")
	if err != nil || got == nil || got.ProposalID != first.ID {
		t.Fatalf("open = %+v, %v", got, err)
	}
}

func TestSettledProposalReleasesChangeKey(t *testing.T) {
	store, c, _ := mergeEnv(t)
	first := insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	mustExec(t, store, `UPDATE coordinator_proposals SET status = 'rejected' WHERE id = '`+first.ID+`'`)
	insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "bbb")
	got, err := store.openMergeForChange(context.Background(), store.db, c.WorkspaceID, "github|github.com|r1|7")
	if err != nil || got == nil || got.HeadSHA != "bbb" {
		t.Fatalf("open = %+v, %v", got, err)
	}
}

func TestUncertainProposalIsOpenAndBlocksTheChange(t *testing.T) {
	store, c, _ := mergeEnv(t)
	first := insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	mustExec(t, store, `UPDATE coordinator_proposals SET status = 'uncertain' WHERE id = '`+first.ID+`'`)
	got, err := store.openMergeForChange(context.Background(), store.db, c.WorkspaceID, "github|github.com|r1|7")
	if err != nil || got == nil || got.ProposalID != first.ID {
		t.Fatalf("uncertain must stay open: %+v, %v", got, err)
	}
	n, err := store.CountOpenProposals(context.Background(), c.ID, true)
	if err != nil || n != 1 {
		t.Fatalf("open count = %d, %v", n, err)
	}
}

func TestOneTaskMayHoldMergeProposalsForSeveralChanges(t *testing.T) {
	store, c, _ := mergeEnv(t)
	insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	insertMergeProposalFor(t, store, c, "github|github.com|r1|8", "bbb")
}

func TestDispatchUniqueOnlyWhileUnsettled(t *testing.T) {
	store, c, _ := mergeEnv(t)
	ctx := context.Background()
	p1 := insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	d := MergeDispatch{ProposalID: p1.ID, WorkspaceID: c.WorkspaceID, ChangeKey: "github|github.com|r1|7", HeadSHA: "aaa", ClaimToken: "tok"}
	if err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return store.insertDispatchTx(ctx, tx, d) }); err != nil {
		t.Fatal(err)
	}
	mustExec(t, store, `UPDATE coordinator_proposals SET status = 'rejected' WHERE id = '`+p1.ID+`'`)
	p2 := insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	d2 := d
	d2.ProposalID = p2.ID
	err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return store.insertDispatchTx(ctx, tx, d2) })
	if err == nil {
		t.Fatal("a second unsettled dispatch for one change and head was inserted")
	}
	if err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		_, err := store.settleDispatchTx(ctx, tx, p1.ID, "refused", "", time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return store.insertDispatchTx(ctx, tx, d2) }); err != nil {
		t.Fatalf("a settled dispatch must release the head: %v", err)
	}
}

func TestDispatchIsAtMostOncePerProposal(t *testing.T) {
	store, c, _ := mergeEnv(t)
	ctx := context.Background()
	p := insertMergeProposalFor(t, store, c, "github|github.com|r1|7", "aaa")
	d := MergeDispatch{ProposalID: p.ID, WorkspaceID: c.WorkspaceID, ChangeKey: "github|github.com|r1|7", HeadSHA: "aaa", ClaimToken: "tok"}
	if err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return store.insertDispatchTx(ctx, tx, d) }); err != nil {
		t.Fatal(err)
	}
	if err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		_, err := store.settleDispatchTx(ctx, tx, p.ID, "refused", "", time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return store.insertDispatchTx(ctx, tx, d) }); err == nil {
		t.Fatal("one proposal recorded two dispatches")
	}
}
