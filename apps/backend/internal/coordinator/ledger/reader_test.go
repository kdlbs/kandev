package ledger

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
)

func (f *fixture) ledgerTurn(id, st string, started time.Time, finished bool, verdict string) {
	f.t.Helper()
	var fin, v any
	if finished {
		fin = started.Add(time.Minute)
	}
	if verdict != "" {
		v = verdict
	}
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at, finished_at, outcome, verdict)
		VALUES (?, ?, ?, ?, 'message', ?, ?, ?, ?)`, id, f.coord.ID, testSession, st, started, fin, map[bool]any{true: "completed", false: nil}[finished], v)
}

func (f *fixture) callRow(turnID, action, target string, allowed bool) {
	f.t.Helper()
	var tgt any
	if target != "" {
		tgt = target
	}
	f.exec(`INSERT INTO coordinator_turn_calls (turn_id, action, target_task_id, allowed, recorded_at) VALUES (?, ?, ?, ?, ?)`, turnID, action, tgt, allowed, f.now)
}

func intp(n int) *int { return &n }

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var fe *coordinator.FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("not a FieldError: %v", err)
	}
	return fe.Field
}

func TestReader_OrderPagingAndCursor(t *testing.T) {
	f := newFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		f.ledgerTurn(fmt.Sprintf("t%d", i), fmt.Sprintf("st%d", i), base.Add(time.Duration(i/2)*time.Minute), true, VerdictActed)
	}
	p, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Limit: intp(2)})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Turns) != 2 || p.Turns[0].ID != "t4" || p.Turns[1].ID != "t3" || p.NextBefore == nil {
		t.Fatalf("page1 = %+v", p)
	}
	p2, _ := f.l.List(t.Context(), f.coord.ID, ListArgs{Limit: intp(2), Before: *p.NextBefore})
	if len(p2.Turns) != 2 || p2.Turns[0].ID != "t2" || p2.Turns[1].ID != "t1" || p2.NextBefore == nil {
		t.Fatalf("page2 = %+v", p2)
	}
	p3, _ := f.l.List(t.Context(), f.coord.ID, ListArgs{Limit: intp(2), Before: *p2.NextBefore})
	if len(p3.Turns) != 1 || p3.Turns[0].ID != "t0" || p3.NextBefore != nil {
		t.Fatalf("page3 = %+v", p3)
	}
}

func TestReader_ValidationNamesTheArgument(t *testing.T) {
	f := newFixture(t)
	cases := map[string]ListArgs{
		"since":   {Since: "yesterday"},
		"limit":   {Limit: intp(0)},
		"verdict": {Verdict: "great"},
		"trigger": {Trigger: "cron"},
		"before":  {Before: "!!"},
	}
	for field, a := range cases {
		_, err := f.l.List(t.Context(), f.coord.ID, a)
		if err == nil || fieldOf(t, err) != field {
			t.Errorf("%s: err = %v", field, err)
		}
	}
	if _, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Limit: intp(51)}); err == nil {
		t.Error("limit 51 accepted")
	}
	old := f.now.Add(-401 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Since: old}); err == nil || fieldOf(t, err) != "since" {
		t.Errorf("since 401d: %v", err)
	}
}

func TestReader_FutureSinceIsEmptyAndFiltersApply(t *testing.T) {
	f := newFixture(t)
	f.ledgerTurn("a", "sa", time.Now().UTC().Add(-time.Hour), true, VerdictActed)
	f.ledgerTurn("b", "sb", time.Now().UTC().Add(-2*time.Hour), false, "")
	p, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Since: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)})
	if err != nil || len(p.Turns) != 0 || p.NextBefore != nil {
		t.Fatalf("future: %+v %v", p, err)
	}
	p, _ = f.l.List(t.Context(), f.coord.ID, ListArgs{Verdict: VerdictActed})
	if len(p.Turns) != 1 || p.Turns[0].ID != "a" {
		t.Fatalf("verdict filter: %+v", p)
	}
	p, _ = f.l.List(t.Context(), f.coord.ID, ListArgs{})
	if p.Turns[1].Verdict != nil || p.Turns[1].FinishedAt != nil || p.Turns[1].Outcome != nil {
		t.Fatalf("unfinished row not null: %+v", p.Turns[1])
	}
}

func TestReader_OnlyOwnCoordinatorRows(t *testing.T) {
	f := newFixture(t)
	f.ledgerTurn("mine", "s1", time.Now().UTC().Add(-time.Hour), true, VerdictActed)
	f.exec(`INSERT INTO coordinator_turns (id, coordinator_id, session_id, session_turn_id, "trigger", started_at) VALUES ('theirs', 'other', 'x', 'y', 'message', ?)`, time.Now().UTC().Add(-time.Hour))
	p, _ := f.l.List(t.Context(), f.coord.ID, ListArgs{})
	if len(p.Turns) != 1 || p.Turns[0].ID != "mine" {
		t.Fatalf("rows = %+v", p.Turns)
	}
}

func TestReader_TaskFilterAndWatchSet(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO tasks (id, workspace_id, workflow_id) VALUES ('in', 'ws-1', 'wf-1'), ('out', 'ws-1', 'wf-2')`)
	f.watch = coordinator.WatchSet{WorkflowIDs: []string{"wf-1"}}
	f.ledgerTurn("a", "sa", time.Now().UTC().Add(-time.Hour), true, VerdictActed)
	f.ledgerTurn("b", "sb", time.Now().UTC().Add(-2*time.Hour), true, VerdictActed)
	f.callRow("a", "move_task", "in", true)
	f.callRow("a", "move_task", "out", false)
	f.callRow("b", "list_tasks", "", true)

	p, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "in"})
	if err != nil || len(p.Turns) != 1 || p.Turns[0].ID != "a" {
		t.Fatalf("task filter: %+v %v", p, err)
	}
	calls := p.Turns[0].Calls
	if len(calls) != 2 || calls[0].Target == nil || *calls[0].Target != "in" || calls[1].Target != nil || calls[1].Allowed {
		t.Fatalf("digest = %+v", calls)
	}
	if _, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "out"}); !errors.Is(err, coordinator.ErrNotFound) {
		t.Fatalf("outside task: %v", err)
	}
	if _, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "missing"}); !errors.Is(err, coordinator.ErrNotFound) {
		t.Fatalf("missing task: %v", err)
	}
}

func TestReader_WatchSetFailureIsUnavailable(t *testing.T) {
	f := newFixture(t)
	f.l.deps.WatchSet = func(context.Context, string) (coordinator.WatchSet, error) { return coordinator.WatchSet{}, errTest }
	if _, err := f.l.List(t.Context(), f.coord.ID, ListArgs{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestReader_ProposalsAndUsage(t *testing.T) {
	f := newFixture(t)
	f.ledgerTurn("a", "sa", time.Now().UTC().Add(-time.Hour), true, VerdictProposed)
	f.ledgerTurn("b", "sb", time.Now().UTC().Add(-2*time.Hour), true, VerdictActed)
	f.ledgerTurn("c", "sc", time.Now().UTC().Add(-3*time.Hour), true, VerdictActed)
	for i := 0; i < 22; i++ {
		f.proposal(fmt.Sprintf("p%02d", i), f.coord.ID, f.now.Add(time.Duration(i)*time.Second))
		f.exec(`UPDATE coordinator_proposals SET turn_id = 'a', target_task_id = ? WHERE id = ?`, fmt.Sprintf("tt%d", i), fmt.Sprintf("p%02d", i))
	}
	f.exec(`INSERT INTO task_usage_events (session_id, turn_id, tokens_in, tokens_out, tokens_total, cost_subcents) VALUES (?, 'sa', 10, 5, 15, 7), (?, 'sa', 1, 1, 2, 3)`, testSession, testSession)
	f.exec(`INSERT INTO task_usage_events (session_id, turn_id, tokens_in, tokens_out, tokens_total, cost_subcents, cost_source) VALUES (?, 'sb', 4, 1, 5, 0, 'unpriced')`, testSession)

	p, err := f.l.List(t.Context(), f.coord.ID, ListArgs{})
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := p.Turns[0], p.Turns[1], p.Turns[2]
	if a.ProposalCount != 22 || len(a.ProposalIDs) != 20 || a.ProposalIDs[0] != "p00" {
		t.Fatalf("proposals = %d %v", a.ProposalCount, a.ProposalIDs)
	}
	if a.Tokens == nil || a.Tokens.Total != 17 || a.CostSubcents == nil || *a.CostSubcents != 10 {
		t.Fatalf("usage a = %+v %v", a.Tokens, a.CostSubcents)
	}
	if b.Tokens == nil || b.CostSubcents != nil {
		t.Fatalf("unpriced must have null cost: %+v %v", b.Tokens, b.CostSubcents)
	}
	if c.Tokens != nil || c.CostSubcents != nil {
		t.Fatalf("no usage rows: %+v %v", c.Tokens, c.CostSubcents)
	}
}

func TestReader_AllWatchScopeStaysInsideTheCoordinatorsWorkspace(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO tasks (id, workspace_id, workflow_id) VALUES ('mine', 'ws-1', 'wf-1'), ('foreign', 'ws-2', 'wf-9')`)
	f.watch = coordinator.WatchSet{All: true}
	f.ledgerTurn("a", "sa", time.Now().UTC().Add(-time.Hour), true, VerdictActed)
	f.callRow("a", "get_task_conversation_kandev", "mine", true)
	f.callRow("a", "get_task_conversation_kandev", "foreign", false)

	_, foreignErr := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "foreign"})
	_, missingErr := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "no-such-task"})
	if !errors.Is(foreignErr, coordinator.ErrNotFound) || !errors.Is(missingErr, coordinator.ErrNotFound) {
		t.Fatalf("foreign = %v, missing = %v, want both not found", foreignErr, missingErr)
	}
	p, err := f.l.List(t.Context(), f.coord.ID, ListArgs{Task: "mine"})
	if err != nil || len(p.Turns) != 1 {
		t.Fatalf("own task: %+v %v", p, err)
	}
	calls := p.Turns[0].Calls
	if len(calls) != 2 || calls[0].Target == nil || *calls[0].Target != "mine" || calls[1].Target != nil {
		t.Fatalf("digest = %+v", calls)
	}
}
