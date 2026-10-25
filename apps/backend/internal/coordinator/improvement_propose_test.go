package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestProposeImprovement_StoresPendingProposalWithContextBefore(t *testing.T) {
	f := newImprovementFixture(t)
	before := improvementCounter(improvementProposed)
	p := f.mustPropose(t)
	if p.Status != ProposalStatusPending || p.Kind != ProposalKindImprovement || p.TargetTaskID != nil {
		t.Fatalf("proposal = %+v", p)
	}
	var spec improvementSpec
	if err := jsonUnmarshal(p.RawSpec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.ContextBefore != "old instructions" || spec.ContextAfter != "new instructions" || len(spec.Evidence) != 2 ||
		spec.Evidence[0].RunID != "run-1" || spec.Evidence[1].TaskID != "task-0" {
		t.Fatalf("spec = %+v", spec)
	}
	rows := listActivity(t, f.store, f.c.ID)
	if len(rows) != 1 || rows[0].ActionClass != ActionImprovement || rows[0].Outcome != ActivityProposed || rows[0].TargetTaskID != nil {
		t.Fatalf("activity = %+v", rows)
	}
	if improvementCounter(improvementProposed) != before+1 {
		t.Fatal("proposed counter did not move")
	}
	if f.context(t) != "old instructions" {
		t.Fatal("propose changed the coordinator")
	}
}

func TestProposeImprovement_Refusals(t *testing.T) {
	long := strings.Repeat("a", 10001)
	cases := []struct {
		name  string
		over  map[string]any
		field string
	}{
		{"title empty", map[string]any{"title": "  "}, "title"},
		{"title long", map[string]any{"title": strings.Repeat("t", 61)}, "title"},
		{"title wrong type", map[string]any{"title": 4}, "title"},
		{"rationale empty", map[string]any{"rationale": ""}, "rationale"},
		{"rationale long", map[string]any{"rationale": long}, "rationale"},
		{"context empty", map[string]any{"context": "   "}, "context"},
		{"context absent", map[string]any{"context": nil}, "context"},
		{"context too long", map[string]any{"context": strings.Repeat("c", 4001)}, "context"},
		{"evidence absent", map[string]any{"evidence": nil}, "evidence"},
		{"evidence empty", map[string]any{"evidence": []any{}}, "evidence"},
		{"evidence eleven", map[string]any{"evidence": elevenRuns()}, "evidence"},
		{"evidence not array", map[string]any{"evidence": "run-1"}, "evidence"},
		{"evidence both keys", map[string]any{"evidence": []map[string]string{{"run_id": "run-1", "task_id": "task-0"}}}, "evidence"},
		{"evidence neither key", map[string]any{"evidence": []map[string]string{{}}}, "evidence"},
		{"evidence extra key", map[string]any{"evidence": []map[string]string{{"run_id": "run-1", "x": "y"}}}, "evidence"},
		{"evidence empty id", map[string]any{"evidence": []map[string]string{{"run_id": " "}}}, "evidence"},
		{"evidence non-object", map[string]any{"evidence": []any{"run-1"}}, "evidence"},
		{"evidence non-string id", map[string]any{"evidence": []any{map[string]any{"run_id": 7}}}, "evidence"},
		{"evidence repeated run", map[string]any{"evidence": []map[string]string{{"run_id": "run-1"}, {"run_id": "run-1"}}}, "evidence"},
		{"evidence repeated task", map[string]any{"evidence": []map[string]string{{"run_id": "run-1"}, {"task_id": "task-0"}, {"task_id": "task-0"}}}, "evidence"},
		{"evidence no run", map[string]any{"evidence": []map[string]string{{"task_id": "task-0"}}}, "evidence"},
		{"evidence unknown run", map[string]any{"evidence": []map[string]string{{"run_id": "nope"}}}, "evidence"},
		{"same context", map[string]any{"context": " old instructions "}, "context"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newImprovementFixture(t)
			_, err := f.propose(t, tc.over)
			wantField(t, err, tc.field)
			if f.proposalCount(t) != 0 || len(listActivity(t, f.store, f.c.ID)) != 0 {
				t.Fatal("a refused call stored something")
			}
		})
	}
}

func elevenRuns() []map[string]string {
	out := make([]map[string]string, 11)
	for i := range out {
		out[i] = map[string]string{"run_id": "run-" + string(rune('a'+i))}
	}
	return out
}

func TestProposeImprovement_EvidenceIsScopedToTheCoordinatorAndWorkspace(t *testing.T) {
	f := newImprovementFixture(t)
	other := newTestCoordinator(t, f.store, "ws-1")
	f.insertRun(t, "foreign-run", other.ID)
	_, err := f.propose(t, map[string]any{"evidence": []map[string]string{{"run_id": "foreign-run"}}})
	wantField(t, err, "evidence")
	foreignMsg := err.Error()
	_, err = f.propose(t, map[string]any{"evidence": []map[string]string{{"run_id": "missing"}}})
	if err == nil || err.Error() != foreignMsg {
		t.Fatalf("absent and foreign runs must read the same: %v vs %q", err, foreignMsg)
	}
	f.tasks.target = &TargetTask{ID: "task-x", WorkspaceID: "ws-2"}
	_, err = f.propose(t, nil)
	wantField(t, err, "evidence")
	f.tasks.err = ErrTaskNotFound
	_, err = f.propose(t, nil)
	wantField(t, err, "evidence")
	if f.proposalCount(t) != 0 {
		t.Fatal("stored despite refused evidence")
	}
}

func TestProposeImprovement_CitedTaskNeedNotBeWatchedOrLive(t *testing.T) {
	f := newImprovementFixture(t)
	archived := timeNow()
	f.tasks.target = &TargetTask{ID: "task-0", WorkspaceID: "ws-1", WorkflowID: "wf-unwatched", ArchivedAt: &archived}
	if _, err := f.propose(t, nil); err != nil {
		t.Fatalf("archived unwatched task must be citable: %v", err)
	}
}

func TestProposeImprovement_PhaseThreeOffStoresNothing(t *testing.T) {
	f := newImprovementFixture(t)
	f.svc.phase3 = false
	_, err := f.propose(t, nil)
	if !errors.Is(err, ErrImprovementsUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if f.proposalCount(t) != 0 || len(listActivity(t, f.store, f.c.ID)) != 0 {
		t.Fatal("stored while phase 3 was off")
	}
}

func TestProposeImprovement_CountsTowardTheOpenLimitAndOrdersRefusals(t *testing.T) {
	f := newImprovementFixture(t)
	for i := 0; i < maxOpenProposals; i++ {
		f.mustPropose(t)
	}
	_, err := f.propose(t, nil)
	if !errors.Is(err, ErrCoordinatorProposalCapReached) {
		t.Fatalf("err = %v, want the limit error", err)
	}
	_, err = f.propose(t, map[string]any{"context": "old instructions"})
	if !errors.Is(err, ErrCoordinatorProposalCapReached) {
		t.Fatalf("equal context over the limit must answer the limit error, got %v", err)
	}
	_, err = f.propose(t, map[string]any{"evidence": []map[string]string{{"task_id": "task-0"}}, "context": "old instructions"})
	wantField(t, err, "evidence")
}

type hookKindTasks struct {
	KindTaskReader
	onGet func()
}

func (h *hookKindTasks) GetTarget(ctx context.Context, id string) (*TargetTask, error) {
	if h.onGet != nil {
		h.onGet()
		h.onGet = nil
	}
	return h.KindTaskReader.GetTarget(ctx, id)
}

func TestProposeImprovement_ContextBeforeIsReadInsideTheLockedTransaction(t *testing.T) {
	f := newImprovementFixture(t)
	// A context edit landing after validation and before the insert makes the
	// after equal the stored context: the proposal is refused, not stored with
	// before == after.
	hook := &hookKindTasks{KindTaskReader: f.tasks, onGet: func() { f.setContext(t, "new instructions") }}
	f.svc.SetKindDeps(KindDeps{Tasks: hook})
	_, err := f.propose(t, nil)
	wantField(t, err, "context")
	if f.proposalCount(t) != 0 || len(listActivity(t, f.store, f.c.ID)) != 0 {
		t.Fatal("stored a proposal whose before equals its after")
	}

	f.setContext(t, "old instructions")
	hook = &hookKindTasks{KindTaskReader: f.tasks, onGet: func() { f.setContext(t, "edited meanwhile") }}
	f.svc.SetKindDeps(KindDeps{Tasks: hook})
	p, err := f.propose(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	var spec improvementSpec
	if err := jsonUnmarshal(p.RawSpec, &spec); err != nil || spec.ContextBefore != "edited meanwhile" {
		t.Fatalf("context_before = %q err=%v, want the context read inside the transaction", spec.ContextBefore, err)
	}
}
