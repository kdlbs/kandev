package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func proposeFixture(t *testing.T) *kindsFixture {
	t.Helper()
	f := newKindsFixture(t)
	createWorkflowsTable(t, f.store)
	addWorkflow(t, f.store, "wf-1", "ws-1")
	tt := idleSession()
	tt.WorkflowID, tt.WorkflowStepID = "wf-1", "step-1"
	f.tasks = &fakeKindTasks{target: tt}
	f.svc.SetKindDeps(KindDeps{Tasks: f.tasks, Messenger: &fakeMessenger{}, Resumer: &fakeResumer{}})
	f.undo.nodes = []StepNode{{ID: "step-1", IsStart: true}, {ID: "manual-step", AllowManualMove: true}}
	return f
}

func (f *kindsFixture) propose(kind, args string, ids ...string) (*Proposal, bool, error) {
	return f.svc.ProposeKind(context.Background(), f.c.ID, kind, json.RawMessage(args), ids)
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	var fe *FieldError
	if !errors.As(err, &fe) || fe.Field != field {
		t.Fatalf("err = %v, want a FieldError naming %q", err, field)
	}
}

func TestProposeKind_TargetRefusals(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		mutate func(*TargetTask)
		kind   string
		args   string
		field  string
	}{
		{"other workspace", func(x *TargetTask) { x.WorkspaceID = "ws-2" }, ProposalKindResume, `{"task_id":"task-0"}`, "task_id"},
		{"archived", func(x *TargetTask) { x.ArchivedAt = &now }, ProposalKindResume, `{"task_id":"task-0"}`, "task_id"},
		{"conversation", func(x *TargetTask) { x.Origin = "coordinator" }, ProposalKindResume, `{"task_id":"task-0"}`, "task_id"},
		{"unwatched", func(x *TargetTask) { x.WorkflowID = "wf-x" }, ProposalKindResume, `{"task_id":"task-0"}`, "task_id"},
		{"resume completed", func(x *TargetTask) { x.Primary.State = "COMPLETED" }, ProposalKindResume, `{"task_id":"task-0"}`, "task_id"},
		{"resume created", func(x *TargetTask) { x.Primary.State = "CREATED" }, ProposalKindResume, `{"task_id":"task-0"}`, "task_id"},
		{"resume idle no record", func(x *TargetTask) { x.Primary.HasExecutorRecord = false }, ProposalKindResume, `{"task_id":"task-0"}`, "task_id"},
		{"message created", func(x *TargetTask) { x.Primary.State = "CREATED" }, ProposalKindMessage, `{"task_id":"task-0","text":"hi"}`, "task_id"},
		{"message empty", func(x *TargetTask) {}, ProposalKindMessage, `{"task_id":"task-0","text":"  "}`, "text"},
		{"message long", func(x *TargetTask) {}, ProposalKindMessage, `{"task_id":"task-0","text":"` + string(make([]byte, 0)) + repeat("a", 4001) + `"}`, "text"},
		{"move same step", func(x *TargetTask) {}, ProposalKindMove, `{"task_id":"task-0","step_id":"step-1"}`, "step_id"},
		{"no task id", func(x *TargetTask) {}, ProposalKindMove, `{"step_id":"manual-step"}`, "task_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := proposeFixture(t)
			f.undo.steps["step-1"] = &UndoStep{WorkflowID: "wf-1"}
			f.undo.steps["manual-step"].WorkflowID = "wf-1"
			mustSave(t, f.svc, f.c.WorkspaceID, f.c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-1"]}}`)
			tc.mutate(f.tasks.target)
			_, _, err := f.propose(tc.kind, tc.args)
			wantField(t, err, tc.field)
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func TestProposeKind_ResumeAcceptsFailedWithoutRecord(t *testing.T) {
	f := proposeFixture(t)
	f.tasks.target.Primary.State, f.tasks.target.Primary.HasExecutorRecord = "FAILED", false
	if _, _, err := f.propose(ProposalKindResume, `{"task_id":"task-0"}`); err != nil {
		t.Fatal(err)
	}
}

func TestProposeKind_MoveStoresStartsAgentAndRefusesWhileDenied(t *testing.T) {
	f := proposeFixture(t)
	f.undo.steps["manual-step"].WorkflowID = "wf-1"
	f.undo.nodes = []StepNode{{ID: "step-1", IsStart: true}, {ID: "manual-step", AllowManualMove: true, AutoStartOnEnter: true}}
	_, _, err := f.propose(ProposalKindMove, `{"task_id":"task-0","step_id":"manual-step"}`)
	wantField(t, err, "step_id")

	mustSave(t, f.svc, f.c.WorkspaceID, f.c.ID, policyBody(map[string]string{"move": "requires_approval", "start_agent": "requires_approval"}))
	p, _, err := f.propose(ProposalKindMove, `{"task_id":"task-0","step_id":"manual-step"}`)
	if err != nil || !p.StartsAgent {
		t.Fatalf("p=%+v err=%v, want stored starts_agent", p, err)
	}
}

func TestProposeKind_MoveIgnoresPlacementClause(t *testing.T) {
	f := proposeFixture(t)
	f.undo.steps["manual-step"].WorkflowID = "wf-1"
	f.undo.nodes = []StepNode{{ID: "step-1", IsStart: true}, {ID: "manual-step"}}
	p, _, err := f.propose(ProposalKindMove, `{"task_id":"task-0","step_id":"manual-step"}`)
	if err != nil || p.StartsAgent {
		t.Fatalf("p=%+v err=%v, want stored with starts_agent false", p, err)
	}
}

func TestProposeKind_DedupeReturnsOpenRowEvenWhenTargetStoppedValidating(t *testing.T) {
	f := proposeFixture(t)
	first, dup, err := f.propose(ProposalKindResume, `{"task_id":"task-0"}`)
	if err != nil || dup {
		t.Fatalf("first: dup=%v err=%v", dup, err)
	}
	now := time.Now()
	f.tasks.target.ArchivedAt = &now
	again, dup, err := f.propose(ProposalKindResume, `{"task_id":"task-0","rationale":"other"}`)
	if err != nil || !dup || again.ID != first.ID {
		t.Fatalf("again=%+v dup=%v err=%v, want the open row with deduplicated", again, dup, err)
	}
	f.forceFailed(t, first.ID)
	again, dup, _ = f.propose(ProposalKindResume, `{"task_id":"task-0"}`)
	if !dup || again.ID != first.ID {
		t.Fatal("a failed proposal must still block a new one")
	}
}

func (f *kindsFixture) forceFailed(t *testing.T, id string) {
	t.Helper()
	if _, err := f.store.db.Exec(f.store.db.Rebind(`UPDATE coordinator_proposals SET status = 'failed' WHERE id = ?`), id); err != nil {
		t.Fatal(err)
	}
}

func TestProposeKind_ConcurrentIdenticalCallsLeaveOneRow(t *testing.T) {
	f := proposeFixture(t)
	var wg sync.WaitGroup
	ids := make(chan string, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if p, _, err := f.propose(ProposalKindResume, `{"task_id":"task-0"}`); err == nil {
				ids <- p.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	var n int
	_ = f.store.db.QueryRow(`SELECT COUNT(*) FROM coordinator_proposals WHERE kind = 'resume'`).Scan(&n)
	if n != 1 || len(seen) != 1 {
		t.Fatalf("rows = %d distinct ids = %d, want 1 and 1", n, len(seen))
	}
}

func TestProposeKind_StandingOrderCitations(t *testing.T) {
	f := proposeFixture(t)
	args := `{"task_id":"task-0"}`
	for _, ids := range [][]string{{"a", "b", "c", "d", "e", "f"}, {"a", "a"}, {""}} {
		_, _, err := f.propose(ProposalKindResume, args, ids...)
		wantField(t, err, "standing_order_ids")
	}
	_, _, err := f.propose(ProposalKindResume, args, "missing")
	wantField(t, err, "standing_order_ids")

	order, err := f.svc.AddStandingOrder(context.Background(), f.c.WorkspaceID, f.c.ID, AddStandingOrderInput{Text: "keep it short"})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := f.propose(ProposalKindResume, args, order.ID)
	if err != nil || len(p.StandingOrderIDs) != 1 {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	var applied *time.Time
	_ = f.store.db.QueryRow(`SELECT last_applied_at FROM coordinator_standing_orders WHERE id = ?`, order.ID).Scan(&applied)
	if applied == nil || !applied.Equal(p.CreatedAt) {
		t.Fatalf("last_applied_at = %v, want the proposal's created_at %v", applied, p.CreatedAt)
	}
}
