package replay

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func prep(t *testing.T, w *world, turn Turn) preparedCase {
	t.Helper()
	pc, err := prepareCase(context.Background(), w, turn)
	if err != nil {
		t.Fatalf("prepareCase: %v", err)
	}
	return pc
}

func baseTurn(w *world, decisions ...Outcome) Turn {
	w.snapshots["s"] = "body"
	w.triggers["t"] = "hello"
	for _, o := range decisions {
		w.proposals[o.ProposalID] = Proposal{Kind: ProposalMove, TargetTaskID: "task-" + o.ProposalID}
		w.titles["task-"+o.ProposalID] = "T"
	}
	return Turn{ID: "t", Trigger: TriggerMessage, SnapshotHash: "s", Outcomes: decisions}
}

func TestSkipPrecedence(t *testing.T) {
	w := newWorld()
	// dream beats a missing snapshot, which beats a missing trigger, which beats no expectation.
	turn := Turn{ID: "x", Trigger: TriggerDream}
	if got := prep(t, w, turn).record.Skip; got != SkipDreamTurn {
		t.Fatalf("dream: %s", got)
	}
	turn = Turn{ID: "x", Trigger: TriggerMessage, SnapshotHash: ""}
	if got := prep(t, w, turn).record.Skip; got != SkipNoSnapshot {
		t.Fatalf("empty hash: %s", got)
	}
	turn.SnapshotHash = "missing"
	if got := prep(t, w, turn).record.Skip; got != SkipNoSnapshot {
		t.Fatalf("no row: %s", got)
	}
	w.snapshots["missing"] = "b"
	if got := prep(t, w, turn).record.Skip; got != SkipInputGone {
		t.Fatalf("no trigger: %s", got)
	}
	w.triggers["x"] = ""
	if got := prep(t, w, turn).record.Skip; got != SkipNoExpectation {
		t.Fatalf("present-but-empty trigger must not skip as gone: %s", got)
	}
}

func TestTitleGoneSkipsAndEmptyTitleDoesNot(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "p1", Decision: DecisionApproved})
	delete(w.titles, "task-p1")
	if got := prep(t, w, turn).record.Skip; got != SkipInputGone {
		t.Fatalf("missing title: %q", got)
	}
	w.titles["task-p1"] = ""
	pc := prep(t, w, turn)
	if pc.record.Skip != "" || len(pc.input.titles) != 1 || pc.input.titles[0].title != "" {
		t.Fatalf("empty title is used as empty: %+v", pc)
	}
}

func TestExpectationClassification(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w,
		Outcome{ProposalID: "a", Decision: DecisionApproved},
		Outcome{ProposalID: "b", Decision: DecisionApproved, EditedFields: []string{"title"}},
		Outcome{ProposalID: "c", Decision: DecisionApproved, Automatic: true},
		Outcome{ProposalID: "d", Decision: DecisionRejected},
		Outcome{ProposalID: "e", Decision: DecisionUndone, Automatic: true},
		Outcome{ProposalID: "f", Decision: DecisionReturned},
		Outcome{ProposalID: "g", Decision: DecisionEdited},
	)
	pc := prep(t, w, turn)
	if len(pc.state.exp.reproduced) != 1 || pc.state.exp.reproduced[0].proposalID != "a" {
		t.Fatalf("reproduced %+v", pc.state.exp.reproduced)
	}
	var avoided []string
	for _, x := range pc.state.exp.avoided {
		avoided = append(avoided, x.proposalID)
	}
	if !reflect.DeepEqual(avoided, []string{"d", "e"}) {
		t.Fatalf("avoided %v (an undone automatic approval is avoided)", avoided)
	}
}

func TestEditedApprovalNeverBlocks(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "b", Decision: DecisionApproved, EditedFields: []string{"title"}})
	if got := prep(t, w, turn).record.Skip; got != SkipNoExpectation {
		t.Fatalf("an approval with edits is no expectation: %q", got)
	}
}

func TestImprovementProposalsAreNeverExpectations(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "i", Decision: DecisionApproved})
	w.proposals["i"] = Proposal{Kind: ProposalImprovement}
	if got := prep(t, w, turn).record.Skip; got != SkipNoExpectation {
		t.Fatalf("improvement: %q", got)
	}
}

func TestProposalGoneContributesNothing(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "a", Decision: DecisionApproved}, Outcome{ProposalID: "gone", Decision: DecisionApproved})
	delete(w.proposals, "gone")
	pc := prep(t, w, turn)
	if len(pc.state.exp.reproduced) != 1 || !reflect.DeepEqual(pc.record.Notes, []string{noteProposalGone}) {
		t.Fatalf("got %+v / %v", pc.state.exp, pc.record.Notes)
	}
}

func TestAmbiguousKeyDroppedFromBothSets(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "a", Decision: DecisionApproved}, Outcome{ProposalID: "b", Decision: DecisionRejected})
	w.proposals["b"] = w.proposals["a"] // same decision key, opposite decisions
	pc := prep(t, w, turn)
	if pc.record.Skip != SkipNoExpectation || !reflect.DeepEqual(pc.record.Notes, []string{noteAmbiguousKey}) {
		t.Fatalf("got skip %q notes %v", pc.record.Skip, pc.record.Notes)
	}
}

func TestSameKeyCountsOnceWithSmallestProposalID(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "z", Decision: DecisionApproved}, Outcome{ProposalID: "m", Decision: DecisionApproved})
	w.proposals["z"] = Proposal{Kind: ProposalMove, TargetTaskID: "same"}
	w.proposals["m"] = Proposal{Kind: ProposalMove, TargetTaskID: "same"}
	w.titles["same"] = "T"
	pc := prep(t, w, turn)
	if len(pc.state.exp.reproduced) != 1 || pc.state.exp.reproduced[0].proposalID != "m" {
		t.Fatalf("got %+v", pc.state.exp.reproduced)
	}
}

func TestCreateTaskContributesNoTitle(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w)
	turn.Outcomes = []Outcome{{ProposalID: "c", Decision: DecisionApproved}}
	w.proposals["c"] = Proposal{Kind: ProposalCreateTask, WorkflowID: "wf", Title: "Do It"}
	pc := prep(t, w, turn)
	if pc.state == nil || len(pc.input.titles) != 0 || pc.state.exp.reproduced[0].key != "wf|do it" {
		t.Fatalf("got %+v", pc)
	}
}

func TestSnapshotReadErrorIsNotASkip(t *testing.T) {
	w := newWorld()
	w.proposalErr = errBoom
	turn := baseTurn(w, Outcome{ProposalID: "a", Decision: DecisionApproved})
	if _, err := prepareCase(context.Background(), w, turn); err == nil {
		t.Fatal("a failed read must stop the replay, not skip the case")
	}
}

func TestWakeTurnHasNoTriggerTextAndPromptCarriesWakeKinds(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "a", Decision: DecisionApproved})
	turn.Trigger, turn.WakeKinds = TriggerWake, []string{"task_done", "stalled"}
	delete(w.triggers, "t")
	pc := prep(t, w, turn)
	if pc.record.Skip != "" {
		t.Fatalf("wake turn skipped %q", pc.record.Skip)
	}
	p := buildPrompt("RENDER", pc.input)
	for _, want := range []string{"RENDER", replayParagraph, "Trigger (wake): task_done, stalled", "body", "task-a: T"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, p)
		}
	}
}

func TestPromptNeverCarriesDecisions(t *testing.T) {
	w := newWorld()
	turn := baseTurn(w, Outcome{ProposalID: "p-secret-id", Decision: DecisionRejected})
	w.proposals["p-secret-id"] = Proposal{Kind: ProposalMove, TargetTaskID: "plain"}
	w.titles["plain"] = "T"
	pc := prep(t, w, turn)
	p := buildPrompt("RENDER", pc.input)
	for _, banned := range []string{"p-secret-id", "rejected", "approved"} {
		if strings.Contains(p, banned) {
			t.Fatalf("prompt leaks %q", banned)
		}
	}
}
