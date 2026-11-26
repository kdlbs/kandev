package coordinator

import (
	"testing"

	"github.com/kandev/kandev/internal/coordinator/outcomes"
)

func TestAnswerRefusalEmitsNoDecisionEventAndApprovalEmitsOne(t *testing.T) {
	fx := newAnswerFx(t)
	obs := &recordingObserver{}
	fx.svc.SetDecisionObserver(obs)

	refused := fx.mustPropose(t, clarificationArgs)
	fx.targets.setSet(clarificationSet("p9", yesNo("q1")))
	if _, err := fx.approveAnswer(refused, nil); !isConflict(err) {
		t.Fatalf("approve after a target change: err = %v", err)
	}
	obs.mu.Lock()
	n := len(obs.events)
	obs.mu.Unlock()
	if n != 0 {
		t.Fatalf("a refusal emitted %d decision events, want none", n)
	}

	fx.targets.setSet(clarificationSet("p1", yesNo("q1"), yesNo("q2")))
	p := fx.mustPropose(t, clarificationArgs)
	if got, err := fx.approveAnswer(p, nil); err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("approve: %+v %v", got, err)
	}
	if e := obs.only(t); e.ProposalKind != ProposalKindAnswer || e.Decision != outcomes.DecisionApproved {
		t.Fatalf("event = %+v", e)
	}
}

func TestApprovedAnswerRowIsNotUndoable(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	if _, err := fx.approveAnswer(p, nil); err != nil {
		t.Fatal(err)
	}
	if undoableClass(ActionAnswer) {
		t.Fatal("answer must not be an undoable class")
	}
	found := false
	for _, r := range listActivity(t, fx.store, fx.c.ID) {
		if r.Outcome == ActivityApproved && r.ActionClass == ActionAnswer {
			found = true
			if undoReadable(&r, map[string]*string{}) {
				t.Fatal("an approved answer row must not offer undo")
			}
		}
	}
	if !found {
		t.Fatal("no approved answer activity row")
	}
}

func TestAnswerProposalsVisibleWhenPhase2OnHiddenWhenOff(t *testing.T) {
	fx := newAnswerFx(t)
	p := fx.mustPropose(t, clarificationArgs)
	if got, err := fx.store.GetProposal(t.Context(), fx.c.WorkspaceID, fx.c.ID, p.ID, true); err != nil || got.ID != p.ID {
		t.Fatalf("phase 2 on: %v %v", got, err)
	}
	if _, err := fx.store.GetProposal(t.Context(), fx.c.WorkspaceID, fx.c.ID, p.ID, false); err == nil {
		t.Fatal("phase 2 off must hide an answer proposal")
	}
}
