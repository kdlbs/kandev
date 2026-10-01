package coordinator

import (
	"context"
	"sync"
	"testing"
)

type fakeLedger struct {
	mu     sync.Mutex
	active map[string]string
	calls  []string
}

func (f *fakeLedger) ActiveTurnID(sessionID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active[sessionID]
}

func (f *fakeLedger) Call(sessionID, action, _ string, allowed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sessionID+"|"+action)
	_ = allowed
}

func (f *fakeLedger) set(sessionID, id string) {
	f.mu.Lock()
	f.active[sessionID] = id
	f.mu.Unlock()
}

func turnIDOf(t *testing.T, s *Store, table, id string) *string {
	t.Helper()
	var v *string
	if err := s.db.QueryRow(s.db.Rebind(`SELECT turn_id FROM `+table+` WHERE id = ?`), id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func linkFixture(t *testing.T) (*Store, *Coordinator, *fakeLedger) {
	t.Helper()
	s := newTestStore(t)
	c := &Coordinator{WorkspaceID: "ws-1", Name: "n", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := s.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	l := &fakeLedger{active: map[string]string{}}
	s.setTurnLedger(l)
	return s, c, l
}

func TestTurnLink_ProposalGetsActiveTurnOfCallerSession(t *testing.T) {
	s, c, l := linkFixture(t)
	l.set("sess", "turn-1")
	ctx := WithCallerSession(context.Background(), "sess")
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: ProposalSpec{Title: "t"}}
	if err := s.InsertProposal(ctx, p, true); err != nil {
		t.Fatal(err)
	}
	if got := turnIDOf(t, s, "coordinator_proposals", p.ID); got == nil || *got != "turn-1" {
		t.Fatalf("turn_id = %v", got)
	}
}

func TestTurnLink_NoSessionNoTurnOrNoLedgerLeavesNull(t *testing.T) {
	s, c, l := linkFixture(t)
	insert := func(ctx context.Context) string {
		p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: ProposalSpec{Title: "t"}}
		if err := s.InsertProposal(ctx, p, true); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	l.set("sess", "turn-1")
	ids := []string{
		insert(context.Background()),                             // a manager or worker path: no caller session
		insert(WithCallerSession(context.Background(), "other")), // session with no open turn
	}
	s.setTurnLedger(nil)
	ids = append(ids, insert(WithCallerSession(context.Background(), "sess"))) // ledger absent
	for _, id := range ids {
		if got := turnIDOf(t, s, "coordinator_proposals", id); got != nil {
			t.Fatalf("proposal %s turn_id = %v, want NULL", id, *got)
		}
	}
}

func TestTurnLink_ActivityOnlyGuardedCallRowsCarryTheTurn(t *testing.T) {
	s, c, l := linkFixture(t)
	l.set("sess", "turn-1")
	ctx := WithCallerSession(context.Background(), "sess")
	write := func(outcome ActivityOutcome, auth ActivityAuthorization, undoOf *string) string {
		row := ActivityRow{ID: string(outcome) + string(auth), CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, ActionClass: ActionCreateTask, Outcome: outcome, Authorization: auth, UndoOfID: undoOf}
		if err := s.InsertActivity(ctx, s.db, row); err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	undoOf := "x"
	want := map[string]bool{
		write(ActivityProposed, AuthRequiresApproval, nil):   true,
		write(ActivityRefused, AuthDenied, nil):              true,
		write(ActivityApproved, AuthRequiresApproval, nil):   false,
		write(ActivityApproved, AuthAutomatic, nil):          false,
		write(ActivityUndone, AuthRequiresApproval, &undoOf): false,
	}
	for id, linked := range want {
		got := turnIDOf(t, s, "coordinator_activity", id)
		if linked != (got != nil) {
			t.Fatalf("row %s turn_id = %v, linked want %v", id, got, linked)
		}
	}
}

func TestTurnLink_RefusalsCoalesceOnlyWithinOneLedgerTurn(t *testing.T) {
	s, c, l := linkFixture(t)
	ctx := context.Background()
	refuse := func(sessionID string) {
		err := s.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
			return s.recordRefusalTx(WithCallerSession(ctx, sessionID), tx, c.ID, c.WorkspaceID, ActionCreateTask, "ceiling", nil)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	count := func() (rows, refusals int) {
		if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(refusal_count), 0) FROM coordinator_activity WHERE reason_code = 'ceiling'`).Scan(&rows, &refusals); err != nil {
			t.Fatal(err)
		}
		return
	}
	l.set("a", "turn-a")
	l.set("b", "turn-b")
	refuse("a")
	refuse("a")
	if rows, refusals := count(); rows != 1 || refusals != 2 {
		t.Fatalf("same turn: rows=%d refusals=%d", rows, refusals)
	}
	refuse("b")
	if rows, _ := count(); rows != 2 {
		t.Fatalf("two attended turns shared one row: %d rows", rows)
	}
	refuse("none") // no open turn: only matches a NULL turn row
	refuse("none")
	if rows, refusals := count(); rows != 3 || refusals != 5 {
		t.Fatalf("null-turn refusals: rows=%d refusals=%d", rows, refusals)
	}
}
