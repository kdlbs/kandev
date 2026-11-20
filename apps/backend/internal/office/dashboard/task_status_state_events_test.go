package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/dashboard"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type statusStateEventRecorder struct {
	subjects []string
}

func (r *statusStateEventRecorder) Publish(_ context.Context, subject string, _ *bus.Event) error {
	r.subjects = append(r.subjects, subject)
	return nil
}

func (*statusStateEventRecorder) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (*statusStateEventRecorder) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (*statusStateEventRecorder) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (*statusStateEventRecorder) Close()            {}
func (*statusStateEventRecorder) IsConnected() bool { return true }

type statusStateChangePublisher struct {
	eventBus bus.EventBus
}

func (p statusStateChangePublisher) PublishCommittedTaskStateChange(
	ctx context.Context, change taskmodels.TaskStateChange, _ bool,
) {
	if p.eventBus == nil || change.Task == nil || !change.Changed {
		return
	}
	data := map[string]interface{}{
		"task_id":      change.Task.ID,
		"workspace_id": change.Task.WorkspaceID,
		"state":        string(change.Task.State),
		"old_state":    string(change.OldState),
		"new_state":    string(change.Task.State),
	}
	_ = p.eventBus.Publish(ctx, events.TaskStateChanged,
		bus.NewEvent(events.TaskStateChanged, "task-service", data))
}

func TestOfficeStatusStateEvents(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "state-event-task", "ws-state-event", "State event", "todo", 0)
	observer := &statusStateEventRecorder{}
	deps.svc.SetEventBus(observer)
	deps.svc.SetTaskStateChangePublisher(statusStateChangePublisher{eventBus: observer})

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "state-event-task",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	count := 0
	for _, subject := range observer.subjects {
		if subject == events.TaskStateChanged {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("task.state_changed publications = %d, want 1 (subjects: %v)", count, observer.subjects)
	}
}
