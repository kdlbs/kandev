package coordinator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/outcomes"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestCommittedRejectIsObservedBeforeCancelledResponseRead(t *testing.T) {
	store, c, _, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	obs := &recordingObserver{}
	svc.SetDecisionObserver(obs)
	ctx, cancel := context.WithCancel(authedContext("manager"))
	defer cancel()
	b := bus.NewMemoryEventBus(svc.logger)
	_, err := b.Subscribe(events.CoordinatorUpdated, func(context.Context, *bus.Event) error { cancel(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	svc.eventBus = b
	_, err = svc.RejectProposal(ctx, c.WorkspaceID, c.ID, p.ID, RejectProposalRequest{Reason: strPtr("manager explanation")})
	if err == nil {
		t.Fatal("post-commit cancelled response read did not fail")
	}
	stored, err := store.GetProposal(t.Context(), c.WorkspaceID, c.ID, p.ID, false)
	if err != nil || stored.Status != ProposalStatusRejected {
		t.Fatalf("reject did not commit: %+v %v", stored, err)
	}
	event := obs.only(t)
	if event.Decision != outcomes.DecisionRejected || event.ActorUserID != "manager" || event.ReasonCode != "other" || !event.At.Equal(stored.UpdatedAt) {
		t.Fatalf("committed rejection evidence changed: %+v", event)
	}
}
