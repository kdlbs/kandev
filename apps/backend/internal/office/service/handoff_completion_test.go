package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// seedHandoffDeliveryTask inserts a delivery task carrying handoff_source
// provenance plus its source office task, both assigned, and returns the
// source task id. The fixture mirrors production writes: the delivery
// task's metadata carries handoff_source{source_task_id, source_workspace_id,
// source_session_id, source_agent_profile_id} (office/runtime/handoff.go),
// and the source task is an office task with a runner participant.
func seedHandoffDeliveryTask(t *testing.T, svc *service.Service, deliveryID, sourceID, state string) {
	t.Helper()
	insertTestTask(t, svc, deliveryID, "ws-1")
	insertTestTask(t, svc, sourceID, "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id IN (?, ?)`, deliveryID, sourceID)
	// The service-test tasks stub has no metadata column; the production
	// office repo owns tasks.metadata. Add the column when it is missing so
	// this file can write handoff_source provenance exactly as the handoff
	// runtime route does.
	svc.ExecSQL(t, `ALTER TABLE tasks ADD COLUMN metadata TEXT DEFAULT '{}'`)
	provenance, err := json.Marshal(map[string]interface{}{
		taskmodels.MetaKeyHandoffSource: map[string]interface{}{
			"source_task_id":          sourceID,
			"source_workspace_id":     "ws-1",
			"source_session_id":       "session-" + sourceID,
			"source_agent_profile_id": "worker-source",
		},
	})
	if err != nil {
		t.Fatalf("marshal handoff source: %v", err)
	}
	svc.ExecSQL(t, `UPDATE tasks SET metadata = ?, state = ? WHERE id = ?`, string(provenance), state, deliveryID)
	setTestTaskAssignee(t, svc, sourceID, "worker-source")
}

func publishTaskStateChanged(t *testing.T, eb bus.EventBus, taskID, oldState, newState string) {
	t.Helper()
	event := bus.NewEvent(events.TaskStateChanged, "test", map[string]string{
		"task_id":   taskID,
		"old_state": oldState,
		"new_state": newState,
	})
	if err := eb.Publish(context.Background(), events.TaskStateChanged, event); err != nil {
		t.Fatalf("publish task state changed: %v", err)
	}
}

func sourceRuns(t *testing.T, svc *service.Service, agentID, reason string) []string {
	t.Helper()
	runs, err := svc.ListRuns(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var out []string
	for _, run := range runs {
		if run.AgentProfileID == agentID && run.Reason == reason {
			out = append(out, run.ID)
		}
	}
	return out
}

func TestHandoffCompletion_ReportsTerminalDeliveryToSource(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	var captured *service.HandoffCompletionSource
	svc.SetHandoffCompletionPolicy(func(_ context.Context, source service.HandoffCompletionSource) (bool, error) {
		value := source
		captured = &value
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-1", "office-source-1", "COMPLETED")

	publishTaskStateChanged(t, eb, "delivery-1", "IN_PROGRESS", "COMPLETED")

	comments, err := svc.ListComments(ctx, "office-source-1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want exactly 1 on the source task", len(comments))
	}
	if !strings.Contains(comments[0].Body, "delivery-1") || !strings.Contains(comments[0].Body, "COMPLETED") {
		t.Fatalf("comment body = %q, want delivery task id and state", comments[0].Body)
	}
	if comments[0].Source != "handoff" {
		t.Fatalf("comment source = %q, want handoff", comments[0].Source)
	}
	if captured == nil {
		t.Fatal("policy was not called")
	}
	if captured.DeliveryTaskID != "delivery-1" || captured.DeliveryWorkspaceID != "ws-1" || captured.DeliveryState != "COMPLETED" {
		t.Fatalf("captured delivery = %+v", captured)
	}
	if captured.SourceTaskID != "office-source-1" || captured.SourceWorkspaceID != "ws-1" ||
		captured.SourceSessionID != "session-office-source-1" || captured.SourceAgentProfileID != "worker-source" ||
		captured.SourceAssigneeAgentProfileID != "worker-source" {
		t.Fatalf("captured source = %+v", captured)
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 1 {
		t.Fatalf("handoff_completed runs = %d, want exactly 1 for the source assignee", len(runs))
	}
	// The report comment must not also queue a generic task_comment wake.
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonTaskComment); len(runs) != 0 {
		t.Fatalf("task_comment runs = %d, want 0 for a handoff completion report", len(runs))
	}
}

func TestHandoffCompletion_NoPolicyProducesNothing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-0", "office-source-0", "COMPLETED")

	publishTaskStateChanged(t, eb, "delivery-0", "IN_PROGRESS", "COMPLETED")

	comments, err := svc.ListComments(ctx, "office-source-0")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 without an opt-in policy", len(comments))
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 0 {
		t.Fatalf("handoff_completed runs = %d, want 0 without an opt-in policy", len(runs))
	}
}

func TestHandoffCompletion_PolicyDenialProducesNothing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return false, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-denied", "office-source-denied", "COMPLETED")

	publishTaskStateChanged(t, eb, "delivery-denied", "IN_PROGRESS", "COMPLETED")

	comments, err := svc.ListComments(ctx, "office-source-denied")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 when the policy denies the report", len(comments))
	}
}

func TestHandoffCompletion_NonTerminalProducesNothing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-2", "office-source-2", "IN_PROGRESS")

	publishTaskStateChanged(t, eb, "delivery-2", "TODO", "IN_PROGRESS")

	comments, err := svc.ListComments(ctx, "office-source-2")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 for a non-terminal state", len(comments))
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 0 {
		t.Fatalf("handoff_completed runs = %d, want 0 for a non-terminal state", len(runs))
	}
}

func TestHandoffCompletion_StaleStateProducesNothing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-stale", "office-source-stale", "COMPLETED")

	// The event names a different terminal state than the stored task, so
	// this is a stale transition and must not report.
	publishTaskStateChanged(t, eb, "delivery-stale", "IN_PROGRESS", "FAILED")

	comments, err := svc.ListComments(ctx, "office-source-stale")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 for a stale transition", len(comments))
	}
}

func TestHandoffCompletion_NoTransitionProducesNothing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-nt", "office-source-nt", "COMPLETED")

	// No transition: the task was already terminal, so this event did not
	// reach the terminal state and must not report.
	publishTaskStateChanged(t, eb, "delivery-nt", "COMPLETED", "COMPLETED")

	comments, err := svc.ListComments(ctx, "office-source-nt")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 without a state transition", len(comments))
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 0 {
		t.Fatalf("handoff_completed runs = %d, want 0 without a state transition", len(runs))
	}
}

func TestHandoffCompletion_RedeliveryAfterReassignmentDoesNotWakeTwice(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	createTestAgent(t, svc, "ws-1", "worker-new")
	seedHandoffDeliveryTask(t, svc, "delivery-5", "office-source-5", "COMPLETED")

	publishTaskStateChanged(t, eb, "delivery-5", "IN_PROGRESS", "COMPLETED")

	// A reassignment between deliveries must not let the same completion
	// wake the new assignee a second time; the run identity is not scoped to
	// the current assignee.
	setTestTaskAssignee(t, svc, "office-source-5", "worker-new")
	publishTaskStateChanged(t, eb, "delivery-5", "IN_PROGRESS", "COMPLETED")

	comments, err := svc.ListComments(ctx, "office-source-5")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want exactly 1 after redelivery", len(comments))
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 1 {
		t.Fatalf("original-assignee handoff_completed runs = %d, want exactly 1", len(runs))
	}
	if runs := sourceRuns(t, svc, "worker-new", service.RunReasonHandoffCompleted); len(runs) != 0 {
		t.Fatalf("new-assignee handoff_completed runs = %d, want 0 after redelivery", len(runs))
	}
}

func TestHandoffCompletion_TaskWithoutProvenanceProducesNothing(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	insertTestTask(t, svc, "plain-task", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "plain-task")

	publishTaskStateChanged(t, eb, "plain-task", "IN_PROGRESS", "COMPLETED")

	comments, err := svc.ListComments(ctx, "plain-task")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 without handoff provenance", len(comments))
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 0 {
		t.Fatalf("handoff_completed runs = %d, want 0 without handoff provenance", len(runs))
	}
}

func TestHandoffCompletion_RedeliveryDoesNotDuplicate(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-4", "office-source-4", "COMPLETED")

	publishTaskStateChanged(t, eb, "delivery-4", "IN_PROGRESS", "COMPLETED")
	publishTaskStateChanged(t, eb, "delivery-4", "IN_PROGRESS", "COMPLETED")

	comments, err := svc.ListComments(ctx, "office-source-4")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want exactly 1 after redelivery", len(comments))
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 1 {
		t.Fatalf("handoff_completed runs = %d, want exactly 1 after redelivery", len(runs))
	}
}

func TestHandoffCompletion_FailedStateReports(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()
	svc.SetHandoffCompletionPolicy(func(context.Context, service.HandoffCompletionSource) (bool, error) {
		return true, nil
	})

	createTestAgent(t, svc, "ws-1", "worker-source")
	seedHandoffDeliveryTask(t, svc, "delivery-3", "office-source-3", "FAILED")

	publishTaskStateChanged(t, eb, "delivery-3", "IN_PROGRESS", "FAILED")

	comments, err := svc.ListComments(ctx, "office-source-3")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 || !strings.Contains(comments[0].Body, "FAILED") {
		t.Fatalf("comments = %+v, want one report naming FAILED", comments)
	}
	if runs := sourceRuns(t, svc, "worker-source", service.RunReasonHandoffCompleted); len(runs) != 1 {
		t.Fatalf("handoff_completed runs = %d, want exactly 1", len(runs))
	}
}
