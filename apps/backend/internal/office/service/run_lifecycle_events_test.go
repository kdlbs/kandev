package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// Pins the lifecycle-event emission contract:
//   - Each successful agent turn under a claimed run lands a "step" event.
//   - AgentCompleted lands a terminal "complete" event.
//   - AgentFailed lands a terminal "error" event.
//
// Combined with the scheduler's "init" + "adapter.invoke" rows, the run
// detail page's Events log gets a stable per-turn sequence in the
// expected order. We also pin that activity rows produced under the
// run's task come back from ListTasksTouchedByRun via the run_id
// column populated by handleAgentTurnMessageSaved's downstream
// LogActivityWithRun calls.
func TestRunLifecycle_StepCompleteEventsEmitted(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	// Queue + claim a run for the task so handleAgentTurnMessageSaved
	// resolves a runID via GetClaimedRunByTaskID.
	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "lifecycle-init",
	); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: %v (run=%v)", err, run)
	}

	// Two agent turns → two step events.
	for i, body := range []string{"first turn", "second turn"} {
		event := bus.NewEvent(events.AgentTurnMessageSaved, "test", map[string]string{
			"task_id":    taskID,
			"session_id": "sess-1",
			"agent_text": body,
			"agent_id":   "worker-1",
		})
		if pErr := eb.Publish(ctx, events.AgentTurnMessageSaved, event); pErr != nil {
			t.Fatalf("publish turn %d: %v", i, pErr)
		}
	}

	// Drive the AgentCompleted handler to emit "complete".
	completed := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id":    taskID,
		"session_id": "sess-1",
	})
	if pErr := eb.Publish(ctx, events.AgentCompleted, completed); pErr != nil {
		t.Fatalf("publish completed: %v", pErr)
	}

	// Two step events + one complete; pin event types in order.
	rowEvents, err := listRunEvents(t, svc, run.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	wantTypes := []string{"step", "step", "complete"}
	if len(rowEvents) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d (%+v)", len(rowEvents), len(wantTypes), rowEvents)
	}
	for i, e := range rowEvents {
		if e.EventType != wantTypes[i] {
			t.Errorf("event[%d].type = %q, want %q", i, e.EventType, wantTypes[i])
		}
		if e.Seq != i {
			t.Errorf("event[%d].seq = %d, want %d (monotonic)", i, e.Seq, i)
		}
	}

	// Tasks Touched should surface the run's task even when the only
	// activity rows came from indirect handlers (status_changed paths
	// log under run_id; per-turn comments don't write activity but
	// the run-detail handler unions in the run payload's task_id).
	// Here we drive a status_changed event under the same task; the
	// service hooks resolveRunForTask + LogActivityWithRun.
	statusEvt := bus.NewEvent(events.OfficeTaskStatusChanged, "test", map[string]interface{}{
		"task_id":      taskID,
		"new_status":   "in_progress",
		"old_status":   "todo",
		"workspace_id": "ws-1",
	})
	if pErr := eb.Publish(ctx, events.OfficeTaskStatusChanged, statusEvt); pErr != nil {
		t.Fatalf("publish status_changed: %v", pErr)
	}
	// The run is now finished (complete handler called FinishRun)
	// so the resolveRunForTask call inside the status handler will
	// no longer find a claimed run. To exercise the persistence path
	// directly, we re-queue + claim once more and seed an activity
	// row tagged with the run id.
	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskComment,
		`{"task_id":"`+taskID+`"}`, "lifecycle-second",
	); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	run2, err := svc.ClaimNextRun(ctx)
	if err != nil || run2 == nil {
		t.Fatalf("claim run2: %v (run=%v)", err, run2)
	}
	svc.LogActivityWithRun(ctx, "ws-1", "agent", "worker-1",
		"task.touched", "task", taskID, "{}", run2.ID, "sess-1")

	tasks, err := tasksTouched(t, svc, run2.ID)
	if err != nil {
		t.Fatalf("tasks touched: %v", err)
	}
	if len(tasks) != 1 || tasks[0] != taskID {
		t.Fatalf("tasks_touched = %v, want [%s]", tasks, taskID)
	}
}

func TestRunLifecycle_ErrorEventEmitted(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, "lifecycle-error",
	); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	failed := bus.NewEvent(events.AgentFailed, "test", map[string]string{
		"task_id":       taskID,
		"session_id":    "sess-err",
		"error_message": "boom",
	})
	if pErr := eb.Publish(ctx, events.AgentFailed, failed); pErr != nil {
		t.Fatalf("publish failed: %v", pErr)
	}

	rowEvents, err := listRunEvents(t, svc, run.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rowEvents) != 1 || rowEvents[0].EventType != "error" {
		t.Fatalf("events = %+v, want single error", rowEvents)
	}
	if rowEvents[0].Level != "error" {
		t.Errorf("level = %q, want error", rowEvents[0].Level)
	}
}

// listRunEvents reads run events via the repo. Exposed via a service
// helper so the integration test stays in the _test package.
func listRunEvents(t *testing.T, svc *service.Service, runID string) ([]*models.RunEvent, error) {
	t.Helper()
	return svc.ListRunEventsForTest(context.Background(), runID)
}

// Pins that AppendRunEvent publishes a per-run-id bus notification
// after writing the row, so the WS gateway can fan it out to clients
// subscribed via run.subscribe. The published event's payload carries
// run_id at the top level + a nested {seq, event_type, level, payload,
// created_at} object — that's the exact shape the broadcaster relays
// as the run.event.appended action.
func TestAppendRunEvent_PublishesBusNotification(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	// Subscribe to the per-run subject before publishing so the
	// memory bus delivers synchronously to our handler.
	const runID = "run-publish-1"
	subject := events.BuildOfficeRunEventSubject(runID)
	got := make(chan *bus.Event, 4)
	sub, err := eb.Subscribe(subject, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	svc.AppendRunEvent(ctx, runID, "init", "info", map[string]interface{}{"k": "v"})

	select {
	case e := <-got:
		data, ok := e.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data not a map: %T", e.Data)
		}
		if data["run_id"] != runID {
			t.Errorf("run_id = %v, want %s", data["run_id"], runID)
		}
		nested, ok := data["event"].(map[string]interface{})
		if !ok {
			t.Fatalf("event payload not a map: %T", data["event"])
		}
		if nested["event_type"] != "init" {
			t.Errorf("event_type = %v, want init", nested["event_type"])
		}
		if nested["level"] != "info" {
			t.Errorf("level = %v, want info", nested["level"])
		}
	default:
		t.Fatalf("expected published event for %s; got none", subject)
	}
}

func tasksTouched(t *testing.T, svc *service.Service, runID string) ([]string, error) {
	t.Helper()
	return svc.ListTasksTouchedByRunForTest(context.Background(), runID)
}

// TestQueueRun_PublishesOfficeRunQueued pins that QueueRun publishes
// an OfficeRunQueued bus event with the {run_id, agent, reason,
// task/comment, idempotency_key} payload the WS gateway relays for
// the per-comment "Queued" badge on the dashboard.
func TestQueueRun_PublishesOfficeRunQueued(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	got := make(chan *bus.Event, 1)
	sub, err := eb.Subscribe(events.OfficeRunQueued, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	const idemKey = "task_comment:cm-123"
	payload := `{"task_id":"` + taskID + `","comment_id":"cm-123"}`
	if err := svc.QueueRun(ctx, "worker-1", service.RunReasonTaskComment, payload, idemKey); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	select {
	case e := <-got:
		data, ok := e.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data not a map: %T", e.Data)
		}
		if data["agent_profile_id"] != "worker-1" {
			t.Errorf("agent_profile_id = %v, want worker-1", data["agent_profile_id"])
		}
		if data["reason"] != service.RunReasonTaskComment {
			t.Errorf("reason = %v, want %s", data["reason"], service.RunReasonTaskComment)
		}
		if data["task_id"] != taskID {
			t.Errorf("task_id = %v, want %s", data["task_id"], taskID)
		}
		if data["comment_id"] != "cm-123" {
			t.Errorf("comment_id = %v, want cm-123", data["comment_id"])
		}
		if data["idempotency_key"] != idemKey {
			t.Errorf("idempotency_key = %v, want %s", data["idempotency_key"], idemKey)
		}
		if data["run_id"] == "" || data["run_id"] == nil {
			t.Errorf("run_id missing: %v", data["run_id"])
		}
	default:
		t.Fatalf("expected OfficeRunQueued event; got none")
	}
}

// TestFinishRun_PublishesOfficeRunProcessed pins that FinishRun
// publishes OfficeRunProcessed with status="finished" plus the same
// {agent, task, comment, reason} fields the WS gateway needs to scope
// the badge update.
func TestFinishRun_PublishesOfficeRunProcessed(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskComment,
		`{"task_id":"`+taskID+`","comment_id":"cm-1"}`, "task_comment:cm-1",
	); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v (run=%v)", err, run)
	}

	got := make(chan *bus.Event, 1)
	sub, err := eb.Subscribe(events.OfficeRunProcessed, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	if err := svc.FinishRun(ctx, run.ID); err != nil {
		t.Fatalf("finish: %v", err)
	}

	select {
	case e := <-got:
		data, ok := e.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data not a map: %T", e.Data)
		}
		if data["run_id"] != run.ID {
			t.Errorf("run_id = %v, want %s", data["run_id"], run.ID)
		}
		if data["status"] != service.RunStatusFinished {
			t.Errorf("status = %v, want %s", data["status"], service.RunStatusFinished)
		}
		if data["task_id"] != taskID {
			t.Errorf("task_id = %v, want %s", data["task_id"], taskID)
		}
		if data["comment_id"] != "cm-1" {
			t.Errorf("comment_id = %v, want cm-1", data["comment_id"])
		}
		if data["agent_profile_id"] != "worker-1" {
			t.Errorf("agent_profile_id = %v, want worker-1", data["agent_profile_id"])
		}
	default:
		t.Fatalf("expected OfficeRunProcessed event; got none")
	}
}

// TestFailRun_PublishesOfficeRunProcessedFailed pins the failed-run
// path: FailRun publishes status="failed" so the comment badge can
// flip to its error state.
func TestFailRun_PublishesOfficeRunProcessedFailed(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-1")
	taskID := createOfficeTask(t, svc, "ws-1", "worker-1")

	if err := svc.QueueRun(
		ctx, "worker-1", service.RunReasonTaskComment,
		`{"task_id":"`+taskID+`","comment_id":"cm-2"}`, "task_comment:cm-2",
	); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v", err)
	}

	got := make(chan *bus.Event, 1)
	sub, err := eb.Subscribe(events.OfficeRunProcessed, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	if err := svc.FailRun(ctx, run.ID); err != nil {
		t.Fatalf("fail: %v", err)
	}

	select {
	case e := <-got:
		data, ok := e.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data not a map: %T", e.Data)
		}
		if data["run_id"] != run.ID {
			t.Errorf("run_id = %v, want %s", data["run_id"], run.ID)
		}
		if data["status"] != service.RunStatusFailed {
			t.Errorf("status = %v, want %s", data["status"], service.RunStatusFailed)
		}
	default:
		t.Fatalf("expected OfficeRunProcessed event for fail; got none")
	}
}
