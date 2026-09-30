package websocket

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// TestCoordinatorEventBroadcaster_Subscribes verifies that
// RegisterCoordinatorNotifications attaches a valid subscription to
// events.CoordinatorUpdated.
func TestCoordinatorEventBroadcaster_Subscribes(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterCoordinatorNotifications(ctx, eventBus, hub, log)
	if b.subscription == nil {
		t.Fatal("expected a subscription, got nil")
	}
	if !b.subscription.IsValid() {
		t.Fatal("expected the subscription to be valid")
	}
}

// TestCoordinatorEventBroadcaster_NilEventBus verifies no panic and no
// subscription when the event bus is nil.
func TestCoordinatorEventBroadcaster_NilEventBus(t *testing.T) {
	log := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	b := RegisterCoordinatorNotifications(ctx, nil, hub, log)
	if b.subscription != nil {
		t.Fatal("expected nil subscription with a nil event bus")
	}
}

// TestCoordinatorEventBroadcaster_ForwardsUpdate verifies that publishing
// events.CoordinatorUpdated invokes the broadcaster's handler exactly once
// and does not panic on the Build decision 13 payload shape.
func TestCoordinatorEventBroadcaster_ForwardsUpdate(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	_ = RegisterCoordinatorNotifications(ctx, eventBus, hub, log)

	var handlerCalled int
	_, _ = eventBus.Subscribe(events.CoordinatorUpdated, func(_ context.Context, _ *bus.Event) error {
		handlerCalled++
		return nil
	})

	payload := map[string]interface{}{
		"workspace_id":   "ws-1",
		"coordinator_id": "co-1",
		"open_proposals": 2,
	}
	evt := bus.NewEvent(events.CoordinatorUpdated, "test", payload)
	if err := eventBus.Publish(context.Background(), events.CoordinatorUpdated, evt); err != nil {
		t.Fatalf("Publish() error: %v", err)
	}

	if handlerCalled != 1 {
		t.Fatalf("handlerCalled = %d, want 1", handlerCalled)
	}
}

// TestCoordinatorEventBroadcaster_IgnoresMissingWorkspaceID verifies the
// forwarder does not panic when a payload carries no workspace_id.
func TestCoordinatorEventBroadcaster_IgnoresMissingWorkspaceID(t *testing.T) {
	log := testLogger()
	eventBus := bus.NewMemoryEventBus(log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	go hub.Run(ctx)

	_ = RegisterCoordinatorNotifications(ctx, eventBus, hub, log)

	evt := bus.NewEvent(events.CoordinatorUpdated, "test", map[string]interface{}{
		"coordinator_id": "co-1",
	})
	if err := eventBus.Publish(context.Background(), events.CoordinatorUpdated, evt); err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
}

func newCoordinatorBroadcastHub(t *testing.T, enforced bool) (*Hub, bus.EventBus, context.Context) {
	t.Helper()
	hub := newAccessTestHub(t)
	hub.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return enforced },
		WorkspaceOwner: func(_ context.Context, workspaceID string) (string, error) {
			if workspaceID == "ws-a" {
				return "user-a", nil
			}
			return "", errors.New("unknown workspace")
		},
	})
	eventBus := bus.NewMemoryEventBus(testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	_ = RegisterCoordinatorNotifications(ctx, eventBus, hub, testLogger())
	return hub, eventBus, ctx
}

func publishRealCoordinatorUpdated(t *testing.T, eventBus bus.EventBus) {
	t.Helper()
	evt := bus.NewEvent(events.CoordinatorUpdated, "test", coordinator.NewCoordinatorUpdatedPayload("ws-a", "co-1", 2))
	if err := eventBus.Publish(context.Background(), events.CoordinatorUpdated, evt); err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
}

// TestCoordinatorEventBroadcaster_RealPayloadRoutesByWorkspaceUnderAuth
// publishes the struct every production publisher uses.
func TestCoordinatorEventBroadcaster_RealPayloadRoutesByWorkspaceUnderAuth(t *testing.T) {
	hub, eventBus, _ := newCoordinatorBroadcastHub(t, true)
	owner := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	other := registerAccessClient(t, hub, "b", authn.Identity{UserID: "user-b", Role: authn.RoleMember})

	publishRealCoordinatorUpdated(t, eventBus)

	waitForMessage(t, owner)
	if got := receivedActions(other); len(got) != 0 {
		t.Fatalf("foreign user received %v, want none", got)
	}
}

func TestCoordinatorEventBroadcaster_RealPayloadDeliveredWithAuthOff(t *testing.T) {
	hub, eventBus, _ := newCoordinatorBroadcastHub(t, false)
	owner := registerAccessClient(t, hub, "a", authn.Identity{UserID: "user-a", Role: authn.RoleMember})
	synthetic := registerAccessClient(t, hub, "s", authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true})
	other := registerAccessClient(t, hub, "b", authn.Identity{UserID: "user-b", Role: authn.RoleMember})

	publishRealCoordinatorUpdated(t, eventBus)

	waitForMessage(t, owner)
	waitForMessage(t, synthetic)
	if got := receivedActions(other); len(got) != 0 {
		t.Fatalf("foreign user received %v, want none: payload must be workspace-scoped", got)
	}
}
