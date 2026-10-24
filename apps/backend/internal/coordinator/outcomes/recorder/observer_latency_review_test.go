package recorder

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/outcomes"
)

type blockedActorChecker struct {
	entered chan struct{}
	release chan struct{}
}

func (c blockedActorChecker) Check(ctx context.Context, _, _ string) (Verdict, error) {
	close(c.entered)
	select {
	case <-c.release:
		return VerdictManager, nil
	case <-ctx.Done():
		return VerdictNotManager, ctx.Err()
	}
}

func TestOverrideCaptureDoesNotHoldCommittedDecision(t *testing.T) {
	f := newFixture(t)
	f.proposal("slow-capture", "rejected", "create_task", "", f.now)
	checker := blockedActorChecker{entered: make(chan struct{}), release: make(chan struct{})}
	o := NewObservers(nil, f.queue(), NewCapture(f.db, checker, nil, nil))
	returned := make(chan struct{})
	go func() {
		o.OnDecision(t.Context(), coordinator.DecisionEvent{ProposalID: "slow-capture",
			CoordinatorID: f.coord.ID, WorkspaceID: f.coord.WorkspaceID, ProposalKind: "create_task",
			ActorUserID: "manager", Decision: outcomes.DecisionRejected, At: f.now})
		close(returned)
	}()
	<-checker.entered
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Error("committed decision waited for override authorization I/O")
	}
	close(checker.release)
	<-returned
	waitFor(t, "manager override capture", func() bool { return f.feedbackCount(FeedbackRejected) == 1 })
	if stoppable, ok := any(o).(interface{ Stop() }); ok {
		stoppable.Stop()
	}
	if got := f.feedbackCount(FeedbackRejected); got != 1 {
		t.Fatalf("released manager override not recorded: %d", got)
	}
}
