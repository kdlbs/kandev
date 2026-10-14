package coordinator

import (
	"context"
	"encoding/json"
	"testing"
)

// TestKindRegistry_NoKindMergesOrReachesADoneStep walks every registered
// executor: each maps to one of the three permitted policy actions (none is a
// merge) or to the activity-only improvement class, and the only executor that names a destination step refuses one that
// completes the task, at propose and again at execute.
func TestKindRegistry_NoKindMergesOrReachesADoneStep(t *testing.T) {
	f := proposeFixture(t)
	f.undo.steps["step-1"] = &UndoStep{WorkflowID: "wf-1"}
	f.undo.steps["manual-step"].WorkflowID = "wf-1"
	f.undo.steps["manual-step"].CompletesOnEnter = true
	mustSave(t, f.svc, f.c.WorkspaceID, f.c.ID, policyBody(map[string]string{"move": "requires_approval", "resume": "requires_approval", "message": "requires_approval"}))

	permitted := map[Action]bool{ActionResume: true, ActionMessage: true, ActionMove: true, ActionImprovement: true}
	execs := f.svc.KindExecutors()
	if len(execs) != len(permitted) {
		t.Fatalf("registry holds %d executors, want %d", len(execs), len(permitted))
	}
	for _, e := range execs {
		if !permitted[e.Action()] {
			t.Fatalf("kind %q maps to action %q, which is not resume, message, move or improvement", e.Kind(), e.Action())
		}
		if e.Kind() == ProposalKindImprovement {
			continue
		}
		args, _ := json.Marshal(map[string]string{"task_id": "task-0", "step_id": "manual-step", "text": "hi"})
		got, _, err := f.svc.ProposeKind(context.Background(), f.c.ID, e.Kind(), args, nil)
		if e.Kind() == ProposalKindMove {
			wantField(t, err, "step_id")
			continue
		}
		if err != nil || got.Status != ProposalStatusPending || got.Kind != e.Kind() {
			t.Fatalf("%s propose: got=%+v err=%v, want a pending proposal of its own kind", e.Kind(), got, err)
		}
		if len(f.undo.moves) != 0 || f.undo.tasks["task-0"].WorkflowStepID != "step-1" {
			t.Fatalf("%s propose touched the task: moves=%d step=%q", e.Kind(), len(f.undo.moves), f.undo.tasks["task-0"].WorkflowStepID)
		}
	}

	f.undo.steps["manual-step"].CompletesOnEnter = false
	p := f.insertMove(t)
	f.undo.steps["manual-step"].CompletesOnEnter = true
	got, err := f.approve(p, nil)
	if err != nil || got.Status != ProposalStatusFailed || len(f.undo.moves) != 0 {
		t.Fatalf("got=%+v err=%v moves=%d, want failed step_is_done with no move", got, err, len(f.undo.moves))
	}
}
