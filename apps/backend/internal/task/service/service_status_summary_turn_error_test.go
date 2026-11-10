package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func TestReconcileExistingSummaryRepairsTurnErrorAttention(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	svc.statusSummaries = repo
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-turn", Name: "Turn Error"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-turn", WorkspaceID: "ws-turn", Name: "Turn Error"})
	occurredAt := time.Date(2026, 10, 29, 20, 32, 55, 0, time.UTC)
	active := models.TaskTurnError{
		StampValue: "failure-1", SessionID: "session-1", MessageID: "failure-1",
		OccurredAt: occurredAt, Message: "Internal error: Failed to execute statement",
	}

	t.Run("missing attention", func(t *testing.T) {
		task := &models.Task{ID: "task-turn-new", WorkspaceID: "ws-turn", WorkflowID: "wf-turn", Title: "new", State: "in_progress", Priority: "medium", CreatedAt: now, UpdatedAt: now, Metadata: map[string]interface{}{
			models.MetaKeyLastTurnError: active,
		}}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		current := statussummary.TaskStatusSummary{Revision: 1}
		if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
			TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: current,
		}); err != nil {
			t.Fatalf("seed summary: %v", err)
		}
		got, err := svc.ReconcileTaskStatusSummaries(
			ctx, []*models.Task{task}, nil, nil,
			map[string]*statussummary.TaskStatusSummary{task.ID: &current},
		)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		summary := got[task.ID]
		if summary == nil || summary.TurnError == nil || summary.TurnError.Stamp != "failure-1" {
			t.Fatalf("summary turn error = %+v, want the durable attention", summary)
		}
		if summary.Revision <= current.Revision {
			t.Fatalf("revision = %d, want a CAS-protected advance from %d", summary.Revision, current.Revision)
		}
	})

	t.Run("dismissal after caller snapshot", func(t *testing.T) {
		acknowledgedAt := occurredAt.Add(time.Minute)
		acknowledged := active
		acknowledged.AcknowledgedAt = &acknowledgedAt
		acknowledged.AcknowledgedBy = models.TurnErrorAcknowledgementDismiss
		task := &models.Task{ID: "task-turn-race", WorkspaceID: "ws-turn", WorkflowID: "wf-turn", Title: "race", State: "in_progress", Priority: "medium", CreatedAt: now, UpdatedAt: now, Metadata: map[string]interface{}{
			models.MetaKeyLastTurnError: active,
		}}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		if err := repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyLastTurnError, acknowledged); err != nil {
			t.Fatalf("seed acknowledged metadata: %v", err)
		}
		current := statussummary.TaskStatusSummary{Revision: 5}
		if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
			TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: current,
		}); err != nil {
			t.Fatalf("seed summary: %v", err)
		}
		got, err := svc.ReconcileTaskStatusSummaries(
			ctx, []*models.Task{task}, nil, nil,
			map[string]*statussummary.TaskStatusSummary{task.ID: &current},
		)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		summary := got[task.ID]
		if summary == nil || summary.TurnError != nil {
			t.Fatalf("summary turn error = %+v, want the acknowledged live metadata preserved", summary)
		}
	})

	t.Run("acknowledged attention", func(t *testing.T) {
		acknowledgedAt := occurredAt.Add(time.Minute)
		acknowledged := active
		acknowledged.AcknowledgedAt = &acknowledgedAt
		acknowledged.AcknowledgedBy = models.TurnErrorAcknowledgementDismiss
		task := &models.Task{ID: "task-turn-ack", WorkspaceID: "ws-turn", WorkflowID: "wf-turn", Title: "ack", State: "in_progress", Priority: "medium", CreatedAt: now, UpdatedAt: now, Metadata: map[string]interface{}{
			models.MetaKeyLastTurnError: acknowledged,
		}}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		current := statussummary.TaskStatusSummary{
			Revision: 3,
			TurnError: &statussummary.TurnErrorSummary{
				SessionID: "session-1", MessageID: "failure-1", Stamp: "failure-1",
				OccurredAt: occurredAt, Preview: "Internal error: Failed to execute statement",
			},
		}
		if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
			TaskID: task.ID, WorkspaceID: task.WorkspaceID, Summary: current,
		}); err != nil {
			t.Fatalf("seed summary: %v", err)
		}
		got, err := svc.ReconcileTaskStatusSummaries(
			ctx, []*models.Task{task}, nil, nil,
			map[string]*statussummary.TaskStatusSummary{task.ID: &current},
		)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		summary := got[task.ID]
		if summary == nil || summary.TurnError != nil {
			t.Fatalf("summary turn error = %+v, want the stale attention cleared", summary)
		}
	})
}
