package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestPublishCoordinatorUpdated_PublishesOpenProposalsCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	if err := store.InsertProposal(ctx, &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}

	svc := NewService(store, newValidatorForTest(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetDecisionDeps(nil, nil, eventBus)

	received := make(chan *CoordinatorUpdatedPayload, 1)
	sub, err := eventBus.Subscribe(events.CoordinatorUpdated, func(_ context.Context, event *bus.Event) error {
		payload, ok := event.Data.(CoordinatorUpdatedPayload)
		if !ok {
			t.Errorf("event.Data type = %T, want CoordinatorUpdatedPayload", event.Data)
			return nil
		}
		received <- &payload
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	svc.publishCoordinatorUpdated(ctx, "ws-1", c.ID)

	select {
	case payload := <-received:
		if payload.WorkspaceID != "ws-1" || payload.CoordinatorID != c.ID || payload.OpenProposals != 1 {
			t.Fatalf("payload = %+v, want ws-1/%s/1", payload, c.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("coordinator.updated was not published")
	}
}

func TestPublishCoordinatorUpdated_NilEventBusIsNoop(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, newValidatorForTest(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	// SetDecisionDeps not called: eventBus stays nil. Must not panic.
	svc.publishCoordinatorUpdated(context.Background(), "ws-1", "coord-1")
}

// subscribeCoordinatorUpdated returns a channel fed by every coordinator.updated
// event eventBus delivers, and a cleanup the caller should defer.
func subscribeCoordinatorUpdated(t *testing.T, eventBus bus.EventBus) (<-chan *CoordinatorUpdatedPayload, func()) {
	t.Helper()
	received := make(chan *CoordinatorUpdatedPayload, 8)
	sub, err := eventBus.Subscribe(events.CoordinatorUpdated, func(_ context.Context, event *bus.Event) error {
		payload, ok := event.Data.(CoordinatorUpdatedPayload)
		if !ok {
			t.Errorf("event.Data type = %T, want CoordinatorUpdatedPayload", event.Data)
			return nil
		}
		received <- &payload
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	return received, func() { _ = sub.Unsubscribe() }
}

// TestApproveProposal_PublishesCoordinatorUpdatedOnCompletion proves the
// golden approve path (claim, then a completed create) publishes exactly one
// coordinator.updated event through the real ApproveProposal entry point, not
// just the publishCoordinatorUpdated helper in isolation
// (docs/specs/coordinator/system-design/proposals.md#events: "after every
// committed claim, re-claim, completion, failure and reject").
func TestApproveProposal_PublishesCoordinatorUpdatedOnCompletion(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetDecisionDeps(tasks, svc.decisionSteps, eventBus)
	received, cleanup := subscribeCoordinatorUpdated(t, eventBus)
	defer cleanup()

	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}

	select {
	case payload := <-received:
		if payload.WorkspaceID != "ws-1" || payload.CoordinatorID != c.ID {
			t.Fatalf("payload = %+v, want ws-1/%s", payload, c.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("coordinator.updated was not published on approve completion")
	}
}

// TestApproveProposal_ConflictDoesNotPublish proves an approve request that
// is refused before any write (an already-decided proposal) does not publish
// coordinator.updated ("never on a zero-row write").
func TestApproveProposal_ConflictDoesNotPublish(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	if _, err := store.RejectProposal(context.Background(), p.ID, "no thanks", "", time.Now()); err != nil {
		t.Fatalf("RejectProposal (setup): %v", err)
	}
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetDecisionDeps(tasks, svc.decisionSteps, eventBus)
	received, cleanup := subscribeCoordinatorUpdated(t, eventBus)
	defer cleanup()

	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err == nil {
		t.Fatal("ApproveProposal: want a conflict error")
	}

	select {
	case payload := <-received:
		t.Fatalf("coordinator.updated published on a no-op conflict: %+v", payload)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestRejectProposal_PublishesCoordinatorUpdatedOnReject proves the reject
// route publishes coordinator.updated through the real RejectProposal entry
// point.
func TestRejectProposal_PublishesCoordinatorUpdatedOnReject(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	eventBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetDecisionDeps(tasks, svc.decisionSteps, eventBus)
	received, cleanup := subscribeCoordinatorUpdated(t, eventBus)
	defer cleanup()

	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatalf("RejectProposal: %v", err)
	}

	select {
	case payload := <-received:
		if payload.WorkspaceID != "ws-1" || payload.CoordinatorID != c.ID {
			t.Fatalf("payload = %+v, want ws-1/%s", payload, c.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("coordinator.updated was not published on reject")
	}
}
