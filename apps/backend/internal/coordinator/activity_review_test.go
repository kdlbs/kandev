package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInsertActivity_KeepsCallerTimestampsAndCount(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := created.Add(time.Hour)
	row := validRow(c.ID)
	row.CreatedAt, row.UpdatedAt, row.RefusalCount = created, updated, 7
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
	got := listActivity(t, store, c.ID)[0]
	if !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(updated) || got.RefusalCount != 7 {
		t.Fatalf("caller values overwritten: %+v", got)
	}
}

func TestInsertActivity_BindsCoordinatorWorkspace(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	row := validRow(c.ID)
	row.WorkspaceID = "ws-foreign"
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
	if got := listActivity(t, store, c.ID)[0]; got.WorkspaceID != "ws-1" {
		t.Fatalf("workspace = %q, want ws-1", got.WorkspaceID)
	}
}

func TestInsertActivity_MissingCoordinatorWritesNothing(t *testing.T) {
	store := newTestStore(t)
	err := store.InsertActivity(context.Background(), store.db, validRow("nope"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRecordRefusal_ForeignWorkspaceBoundToReal(t *testing.T) {
	store := newTestStore(t)
	svc := newPhase2Service(t, store, true)
	c := newTestCoordinator(t, store, "ws-1")
	if err := svc.RecordRefusal(context.Background(), c.ID, "ws-foreign", ActionMove, "denied"); err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].WorkspaceID != "ws-1" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestProposeTask_RecordsProposedActivity(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.phase2 = true
	p, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, f.svc.store, f.coordinator.ID)
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	r := rows[0]
	if r.Outcome != ActivityProposed || r.Authorization != AuthRequiresApproval || r.ActionClass != ActionCreateTask ||
		r.ProposalID == nil || *r.ProposalID != p.ID || r.WorkspaceID != f.workspaceID {
		t.Fatalf("row = %+v", r)
	}
}

func TestProposeTask_FlagOffWritesNoActivity(t *testing.T) {
	f := newProposalTestFixture(t)
	if _, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest()); err != nil {
		t.Fatal(err)
	}
	if n := len(listActivity(t, f.svc.store, f.coordinator.ID)); n != 0 {
		t.Fatalf("rows = %d, want 0", n)
	}
}

func TestProposeTask_RecordFailureRollsBackInsert(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.phase2 = true
	// Dropping the table makes Record fail inside the insert transaction.
	if _, err := f.svc.store.db.Exec(`DROP TABLE coordinator_activity`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest()); err == nil {
		t.Fatal("want error")
	}
	n, err := f.svc.store.CountOpenProposals(context.Background(), f.coordinator.ID, true)
	if err != nil || n != 0 {
		t.Fatalf("open = %d err=%v, want proposal rolled back", n, err)
	}
}
