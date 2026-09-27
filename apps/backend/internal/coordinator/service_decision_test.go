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
