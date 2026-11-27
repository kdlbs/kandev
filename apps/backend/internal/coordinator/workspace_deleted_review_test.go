package coordinator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestWorkspaceDeletedCleanupSurvivesRequestCancellation(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	seedWorkspaceCoordinatorState(t, store, "ws-1")
	seedWorkspaceCoordinatorState(t, store, "ws-2")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	h := workspaceDeletedSubscriber{svc: svc, logger: newTestLogger(t)}
	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := h.handle(ctx, event); err != nil {
		t.Fatalf("committed workspace deletion cleanup abandoned: %v", err)
	}
	coordinators, proposals, stalls := countWorkspaceCoordinatorState(t, store, "ws-1")
	if coordinators != 0 || proposals != 0 || stalls != 0 {
		t.Fatalf("deleted workspace state retained: %d/%d/%d", coordinators, proposals, stalls)
	}
	coordinators, proposals, stalls = countWorkspaceCoordinatorState(t, store, "ws-2")
	if coordinators != 1 || proposals != 5 || stalls != 1 {
		t.Fatalf("unrelated workspace state changed: %d/%d/%d", coordinators, proposals, stalls)
	}
}
