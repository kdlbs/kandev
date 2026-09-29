package coordinator

import (
	"context"
	"errors"
	"testing"
)

func phase2Fixture(t *testing.T, on bool) (*Store, *Coordinator, *Service) {
	t.Helper()
	store, c, tasks, svc := approveFixture(t)
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	svc.phase2 = on
	return store, c, svc
}

func insertKind(t *testing.T, store *Store, c *Coordinator, kind string) *Proposal {
	t.Helper()
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec(), Kind: kind}
	if err := store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	return p
}

func TestApprove_Phase2RecordsApprovedActivity(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].Outcome != ActivityApproved || rows[0].ActionClass != ActionCreateTask {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].ProposalID == nil || *rows[0].ProposalID != p.ID || got.TaskID == nil || rows[0].TargetTaskID == nil || *rows[0].TargetTaskID != *got.TaskID {
		t.Fatalf("row does not point at proposal/task: %+v", rows[0])
	}
}

func TestApprove_Phase1WritesNoActivity(t *testing.T) {
	store, c, svc := phase2Fixture(t, false)
	p := insertProposal(t, store, c, sampleSpec())
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if rows := listActivity(t, store, c.ID); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}

func TestReject_Phase2RecordsRejectedActivity(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	reason := "too big"
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{Reason: &reason}); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].Outcome != ActivityRejected || rows[0].Detail != "too big" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestReject_Phase2RaceWritesNoSecondRow(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err == nil {
		t.Fatal("second reject succeeded")
	}
	if rows := listActivity(t, store, c.ID); len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
}

func TestDecisions_UnknownKindWritesNothing(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	if _, err := store.db.Exec(store.db.Rebind(`UPDATE coordinator_proposals SET kind = ? WHERE id = ?`), "teleport", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); !errors.Is(err, ErrUnknownProposalKind) {
		t.Fatalf("approve err = %v", err)
	}
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); !errors.Is(err, ErrUnknownProposalKind) {
		t.Fatalf("reject err = %v", err)
	}
	var status string
	if err := store.db.Get(&status, store.db.Rebind(`SELECT status FROM coordinator_proposals WHERE id = ?`), p.ID); err != nil || status != string(ProposalStatusPending) {
		t.Fatalf("status = %q err=%v", status, err)
	}
	if rows := listActivity(t, store, c.ID); len(rows) != 0 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestReject_Phase2OnDeletedCoordinatorIs404(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	if err := store.DeleteCoordinator(context.Background(), c.WorkspaceID, c.ID); err != nil {
		t.Fatal(err)
	}
	matched, err := svc.rejectProposalStore(context.Background(), p, "", "")
	if err != nil || matched {
		t.Fatalf("matched=%v err=%v, want false/nil", matched, err)
	}
	if _, err := svc.claimRaceResult(context.Background(), "ws-1", c.ID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
