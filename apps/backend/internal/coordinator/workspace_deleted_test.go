package coordinator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestSubscribeWorkspaceDeleted_DeletesWorkspaceCoordinatorState(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	memBus := bus.NewMemoryEventBus(newTestLogger(t))

	seedWorkspaceCoordinatorState(t, store, "ws-1")
	seedWorkspaceCoordinatorState(t, store, "ws-2")

	if _, err := SubscribeWorkspaceDeleted(memBus, svc, newTestLogger(t)); err != nil {
		t.Fatalf("SubscribeWorkspaceDeleted: %v", err)
	}

	evt := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := memBus.Publish(ctx, events.WorkspaceDeleted, evt); err != nil {
		t.Fatalf("Publish workspace.deleted: %v", err)
	}

	coordinators, proposals, stalls := countWorkspaceCoordinatorState(t, store, "ws-1")
	if coordinators != 0 || proposals != 0 || stalls != 0 {
		t.Fatalf("ws-1 after workspace.deleted: coordinators=%d proposals=%d stalls=%d, want all 0", coordinators, proposals, stalls)
	}

	coordinators, proposals, stalls = countWorkspaceCoordinatorState(t, store, "ws-2")
	if coordinators != 1 || proposals != 5 || stalls != 1 {
		t.Fatalf("ws-2 after ws-1's workspace.deleted: coordinators=%d proposals=%d stalls=%d, want 1/5/1 (untouched)", coordinators, proposals, stalls)
	}
}

func TestSubscribeWorkspaceDeleted_RedeliveredOrEmptyIDIsHarmless(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	ctx := context.Background()
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	seedWorkspaceCoordinatorState(t, store, "ws-1")

	if _, err := SubscribeWorkspaceDeleted(memBus, svc, newTestLogger(t)); err != nil {
		t.Fatalf("SubscribeWorkspaceDeleted: %v", err)
	}

	missingID := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-missing"})
	if err := memBus.Publish(ctx, events.WorkspaceDeleted, missingID); err != nil {
		t.Fatalf("Publish workspace.deleted (missing workspace): %v", err)
	}

	emptyID := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": ""})
	if err := memBus.Publish(ctx, events.WorkspaceDeleted, emptyID); err != nil {
		t.Fatalf("Publish workspace.deleted (empty id): %v", err)
	}

	malformed := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"other": "field"})
	if err := memBus.Publish(ctx, events.WorkspaceDeleted, malformed); err != nil {
		t.Fatalf("Publish workspace.deleted (malformed): %v", err)
	}

	deleteEvt := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := memBus.Publish(ctx, events.WorkspaceDeleted, deleteEvt); err != nil {
		t.Fatalf("Publish workspace.deleted (ws-1): %v", err)
	}
	if err := memBus.Publish(ctx, events.WorkspaceDeleted, deleteEvt); err != nil {
		t.Fatalf("Publish workspace.deleted (ws-1, redelivered): %v", err)
	}

	coordinators, proposals, stalls := countWorkspaceCoordinatorState(t, store, "ws-1")
	if coordinators != 0 || proposals != 0 || stalls != 0 {
		t.Fatalf("ws-1 after repeated workspace.deleted: coordinators=%d proposals=%d stalls=%d, want all 0", coordinators, proposals, stalls)
	}
}
