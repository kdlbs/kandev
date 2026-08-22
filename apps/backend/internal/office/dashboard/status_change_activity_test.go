package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/dashboard"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// hasStatusChangeActivity reports whether the office_activity_log already
// contains a task_status_changed row for taskID.
func hasStatusChangeActivity(t *testing.T, deps *testDeps, taskID, wsID string) bool {
	t.Helper()
	entries, err := deps.repo.ListActivityEntriesByTarget(context.Background(), wsID, taskID, 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, e := range entries {
		if e.Action == "task_status_changed" {
			return true
		}
	}
	return false
}

// TestUpdateTaskStatus_LogsActivitySynchronously pins that UpdateTaskStatus
// writes the task_status_changed activity row itself, in-process, rather
// than depending on an event-bus subscriber to do it. This DashboardService
// (built by newTestDeps) has no event bus wired at all, so before the fix
// (the write lived solely in office/service.Service.handleTaskStatusChanged,
// an OfficeTaskStatusChanged subscriber) this would never produce a row: the
// event is never published without a bus, and nothing bridges to the
// subscriber outside DashboardService anyway. If this activity write ever
// moves back behind an event-bus round-trip, this test catches it.
func TestUpdateTaskStatus_LogsActivitySynchronously(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "sync-task", "ws-sync", "Sync Task", "todo", 0)

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "sync-task",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	if !hasStatusChangeActivity(t, deps, "sync-task", "ws-sync") {
		t.Fatal("expected task_status_changed activity row immediately after UpdateTaskStatus returns")
	}
}

// LogTaskStateChange is the task-service seam used by workflow moves. It must
// write the same durable projection as the Office status endpoint.
func TestLogTaskStateChange_LogsOfficeTask(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "workflow-task", "ws-workflow", "Workflow Task", "in_progress", 0)
	if _, err := deps.db.Exec(`INSERT INTO workspaces (id, office_workflow_id) VALUES ('ws-workflow', 'wf-office')`); err != nil {
		t.Fatalf("create Office workspace: %v", err)
	}
	if _, err := deps.db.Exec(`UPDATE tasks SET workflow_id = 'wf-office' WHERE id = 'workflow-task'`); err != nil {
		t.Fatalf("attach task to Office workflow: %v", err)
	}

	deps.svc.LogTaskStateChange(context.Background(), &taskmodels.Task{
		ID: "workflow-task", WorkspaceID: "ws-workflow", State: v1.TaskStateCompleted,
	}, v1.TaskStateInProgress)

	if !hasStatusChangeActivity(t, deps, "workflow-task", "ws-workflow") {
		t.Fatal("expected task_status_changed activity row for a workflow state transition")
	}
}

// orderingEventBus is a minimal bus.EventBus fake that, on Publish of
// OfficeTaskStatusChanged, snapshots whether the activity row is already
// persisted. Every other EventBus method is a harmless no-op; this fix
// only touches the Publish ordering relative to the activity write.
type orderingEventBus struct {
	checkRow                 func() bool
	activityPresentAtPublish bool
	published                bool
}

func (b *orderingEventBus) Publish(_ context.Context, subject string, _ *bus.Event) error {
	if subject == events.OfficeTaskStatusChanged {
		b.published = true
		b.activityPresentAtPublish = b.checkRow()
	}
	return nil
}

func (b *orderingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *orderingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *orderingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (b *orderingEventBus) Close()            {}
func (b *orderingEventBus) IsConnected() bool { return true }

// TestUpdateTaskStatus_LogsActivityBeforePublishingEvent reproduces the P1
// race from PR fixup round 1: under a NATS-backed bus, the activity-log
// write and the WS broadcast that triggers the frontend's task-detail
// refetch were two independent async subscribers to the same
// OfficeTaskStatusChanged event, with no ordering guarantee between them —
// a browser GET could land before the activity row existed and see stale
// Started/Completed values with nothing left to trigger a corrective
// refetch. The fix sequences the activity write before the publish call
// inside UpdateTaskStatus itself, so the row is guaranteed durable before
// the event (and therefore any broadcast subscriber) can ever fire,
// independent of the bus implementation. This test proves that ordering
// directly: by the time Publish(OfficeTaskStatusChanged) is observed, the
// activity row must already be queryable.
func TestUpdateTaskStatus_LogsActivityBeforePublishingEvent(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "race-task", "ws-race", "Race Task", "todo", 0)

	fake := &orderingEventBus{}
	fake.checkRow = func() bool {
		return hasStatusChangeActivity(t, deps, "race-task", "ws-race")
	}
	deps.svc.SetEventBus(fake)

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "race-task",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	if !fake.published {
		t.Fatal("expected OfficeTaskStatusChanged to be published")
	}
	if !fake.activityPresentAtPublish {
		t.Fatal("activity row was not yet persisted when the status-changed event was published: " +
			"the WS broadcast (and any refetch it triggers) could race ahead of the activity write")
	}
}
