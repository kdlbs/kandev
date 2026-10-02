package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestSubscribeWorkflowDeletedRemovesWatchRowsAndPublishesUpdate(t *testing.T) {
	svc, store := newTestServiceForSubscribers(t)
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetDecisionDeps(nil, nil, memBus)
	c := newTestCoordinator(t, store, "ws-1")
	if _, err := store.db.Exec(store.db.Rebind(`INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, 'wf-gone', ?, ?)`), c.ID, c.WorkspaceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	capture := &captureCoordinatorUpdated{}
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, capture.record); err != nil {
		t.Fatal(err)
	}
	if _, err := SubscribeWorkflowDeleted(memBus, svc, newTestLogger(t)); err != nil {
		t.Fatal(err)
	}

	if err := memBus.Publish(context.Background(), events.WorkflowDeleted, bus.NewEvent(events.WorkflowDeleted, "test", map[string]interface{}{"id": "wf-gone"})); err != nil {
		t.Fatal(err)
	}

	var rows int
	_ = store.db.Get(&rows, `SELECT COUNT(*) FROM coordinator_watches WHERE workflow_id = 'wf-gone'`)
	if rows != 0 || len(capture.snapshot()) != 1 {
		t.Fatalf("rows = %d, updates = %d, want 0 and 1", rows, len(capture.snapshot()))
	}
}
