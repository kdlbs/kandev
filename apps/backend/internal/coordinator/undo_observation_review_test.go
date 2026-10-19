package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/outcomes"
)

func TestCommittedUndoIsObservedAfterRequestCancellation(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Spec: sampleSpec()}
	if err := store.InsertProposal(t.Context(), p, true); err != nil {
		t.Fatal(err)
	}
	seedApproved(t, store, c, "undo-row", ActionCreateTask, time.Now().UTC(), func(r *ActivityRow) { r.ProposalID = &p.ID })
	at := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := store.db.Exec(`UPDATE coordinator_activity SET undone_at = ? WHERE id = ?`, at, "undo-row"); err != nil {
		t.Fatal(err)
	}
	row, err := store.GetActivityRow(t.Context(), store.db, c.ID, "undo-row")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(authedContext("manager"))
	cancel()
	svc.notifyUndone(ctx, row)
	e := obs.only(t)
	if e.Decision != outcomes.DecisionUndone || e.ActorUserID != "manager" || !e.At.Equal(at) {
		t.Fatalf("committed undo or identity lost: %+v", e)
	}
}
