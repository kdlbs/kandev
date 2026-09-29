package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestSettleDecision_ActivityContent(t *testing.T) {
	ctx := context.Background()

	t.Run("approved edited", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p := insertKind(t, store, c, ProposalKindCreateTask)
		edited := sampleSpec()
		edited.Title = "A different title"
		claimDirectly(t, store, p, "tok", edited, time.Now())
		if m, err := svc.completeProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, "tok", "task-x"); err != nil || !m {
			t.Fatalf("matched=%v err=%v", m, err)
		}
		rows := listActivity(t, store, c.ID)
		if len(rows) != 1 || !rows[0].Edited {
			t.Fatalf("rows = %+v, want one edited row", rows)
		}
	})

	t.Run("approved unedited", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p, tok := claimedPhase2(t, store, c)
		if m, err := svc.completeProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "task-x"); err != nil || !m {
			t.Fatalf("matched=%v err=%v", m, err)
		}
		if rows := listActivity(t, store, c.ID); len(rows) != 1 || rows[0].Edited {
			t.Fatalf("rows = %+v, want one unedited row", rows)
		}
	})

	t.Run("failed carries reason code and error", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p, tok := claimedPhase2(t, store, c)
		if m, err := svc.failProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "boom"); err != nil || !m {
			t.Fatalf("matched=%v err=%v", m, err)
		}
		rows := listActivity(t, store, c.ID)
		if len(rows) != 1 || rows[0].Outcome != ActivityFailed || rows[0].Detail != "boom" ||
			rows[0].ReasonCode == nil || *rows[0].ReasonCode != approvalFailedCode {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("rejected carries reason code", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p := insertKind(t, store, c, ProposalKindCreateTask)
		if m, err := svc.rejectProposalStore(ctx, p, "not now", "u1"); err != nil || !m {
			t.Fatalf("matched=%v err=%v", m, err)
		}
		rows := listActivity(t, store, c.ID)
		if len(rows) != 1 || rows[0].ReasonCode == nil || *rows[0].ReasonCode != "not now" || rows[0].Detail != "not now" {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("rejected without reason has no reason code", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p := insertKind(t, store, c, ProposalKindCreateTask)
		if _, err := svc.rejectProposalStore(ctx, p, "", "u1"); err != nil {
			t.Fatal(err)
		}
		if rows := listActivity(t, store, c.ID); len(rows) != 1 || rows[0].ReasonCode != nil {
			t.Fatalf("rows = %+v", rows)
		}
	})
}

func TestProposeTask_ProposedActivityCarriesTitle(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.phase2 = true
	req := f.baseRequest()
	if _, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req); err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, f.svc.store, f.coordinator.ID)
	if len(rows) != 1 || rows[0].Detail == "" {
		t.Fatalf("rows = %+v, want detail = proposal title", rows)
	}
}

func TestPolicyAllows_NonPolicyActionNeverAllowed(t *testing.T) {
	p := Policy{Actions: map[Action]Setting{ActionUnknown: SettingAutomatic, Action("bogus"): SettingRequiresApproval}}
	if p.Allows(ActionUnknown) || p.Allows(Action("bogus")) {
		t.Fatal("an action outside the six must never be allowed")
	}
}

func TestInsertActivity_UnknownClassReadBack(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	row := validRow(c.ID)
	row.ActionClass = ActionUnknown
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].ActionClass != ActionUnknown {
		t.Fatalf("rows = %+v", rows)
	}
}
