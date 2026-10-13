package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func (f *deliverFixture) refuse(t *testing.T, reason string) {
	t.Helper()
	if err := f.svc.RecordRefusal(context.Background(), f.c.ID, f.c.WorkspaceID, ActionMove, reason); err != nil {
		t.Fatal(err)
	}
}

// stamps returns each activity row's unattended_turn_id, oldest first; a null
// stamp is the empty string.
func stamps(t *testing.T, store *Store, coordinatorID string) []string {
	t.Helper()
	var got []sql.NullString
	err := store.db.SelectContext(context.Background(), &got, store.db.Rebind(
		`SELECT unattended_turn_id FROM coordinator_activity WHERE coordinator_id = ? ORDER BY created_at, id`), coordinatorID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(got))
	for i, v := range got {
		out[i] = v.String
	}
	return out
}

func wantStamps(t *testing.T, store *Store, coordinatorID string, want ...string) {
	t.Helper()
	got := stamps(t, store, coordinatorID)
	if len(got) != len(want) {
		t.Fatalf("stamps = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stamps = %q, want %q", got, want)
		}
	}
}

func TestRecordRefusal_StampsTheOpenUnattendedTurn(t *testing.T) {
	f := newDeliverFixture(t)
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.active.turns[ceilingSession] = "st"
	f.refuse(t, "denied")
	wantStamps(t, f.store, f.c.ID, "ut")
}

func TestRecordRefusal_StampsNothingOutsideAnUnattendedTurn(t *testing.T) {
	cases := []struct {
		name  string
		open  *openTurn
		setup func(f *deliverFixture)
	}{
		{"no turn row", nil, nil},
		{"manager turn active", &openTurn{id: "ut", sessionTurn: "st"}, func(f *deliverFixture) { f.active.turns[ceilingSession] = "manager-turn" }},
		{"no active turn", &openTurn{id: "ut", sessionTurn: "st"}, nil},
		{"unbound row", &openTurn{id: "ut"}, func(f *deliverFixture) { f.active.turns[ceilingSession] = "st" }},
		{"unreadable active turn", &openTurn{id: "ut", sessionTurn: "st"}, func(f *deliverFixture) { f.active.err = errors.New("boom") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeliverFixture(t)
			if tc.open != nil {
				f.openTurnRow(t, *tc.open)
			}
			if tc.setup != nil {
				tc.setup(f)
			}
			f.refuse(t, "denied")
			wantStamps(t, f.store, f.c.ID, "")
		})
	}
}

func TestRecordRefusal_CoalescesOnlyWithinTheSameTurn(t *testing.T) {
	f := newDeliverFixture(t)
	f.openTurnRow(t, openTurn{id: "ut", sessionTurn: "st"})
	f.active.turns[ceilingSession] = "st"
	f.refuse(t, "denied")
	f.refuse(t, "denied")
	rows := listActivity(t, f.store, f.c.ID)
	if len(rows) != 1 || rows[0].RefusalCount != 2 {
		t.Fatalf("same turn: rows = %+v, want one row counted twice", rows)
	}
	f.active.turns[ceilingSession] = "manager-turn"
	f.refuse(t, "denied")
	wantStamps(t, f.store, f.c.ID, "ut", "")
}

func TestProposeTask_StampsTheOpenUnattendedTurn(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.phase2, f.svc.phase3 = true, true
	active := &fakeActiveTurns{turns: map[string]string{"sess": "st"}}
	f.svc.SetSpendDeps(nil, active, nil)
	mustExec(t, f.svc.store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, session_turn_id, wake_count, start_ceiling_subcents, started_at)
		VALUES ('ut', ?, 'conv', 'sess', 'st', 1, 100, ?)`, f.coordinator.ID, f.svc.store.now().UTC())
	if _, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest()); err != nil {
		t.Fatal(err)
	}
	wantStamps(t, f.svc.store, f.coordinator.ID, "ut")
}

func TestProposeTask_StampsNothingWithoutAnOpenTurn(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.phase2 = true
	if _, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, f.baseRequest()); err != nil {
		t.Fatal(err)
	}
	wantStamps(t, f.svc.store, f.coordinator.ID, "")
}
