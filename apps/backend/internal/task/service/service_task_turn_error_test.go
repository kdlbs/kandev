package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func seedTurnErrorTask(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID, workspaceID string) {
	t.Helper()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + taskID, WorkspaceID: workspaceID, Name: "wf"})
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: "wf-" + taskID,
		Title: taskID, State: "in_progress", Priority: "medium", CreatedAt: now, UpdatedAt: now,
	}))
}

func TestDismissTaskTurnError(t *testing.T) {
	ctx := context.Background()
	svc, eventBus, repo := createTestService(t)
	seedTurnErrorTask(t, ctx, repo, "task-turn-error", "ws-turn-error")
	record := models.TaskTurnError{
		StampValue: "stamp-1", SessionID: "session-1", TurnID: "turn-1", MessageID: "stamp-1",
		OccurredAt: time.Now().UTC().Add(-time.Minute), Message: "Failed to execute statement",
	}
	stored, err := svc.RecordTaskTurnErrorAttention(ctx, "task-turn-error", record)
	require.NoError(t, err)
	require.True(t, stored)

	err = svc.DismissTaskTurnError(ctx, "task-turn-error", "stale-stamp")
	require.ErrorIs(t, err, ErrTurnErrorStale)

	err = svc.DismissTaskTurnError(ctx, "task-turn-error", "stamp-1")
	require.NoError(t, err)
	err = svc.DismissTaskTurnError(ctx, "task-turn-error", "stamp-1")
	require.NoError(t, err, "repeat dismissal is idempotent")

	task, err := repo.GetTask(ctx, "task-turn-error")
	require.NoError(t, err)
	current, ok := models.LoadTaskTurnError(task.Metadata)
	require.True(t, ok)
	require.True(t, current.Acknowledged())
	require.Equal(t, models.TurnErrorAcknowledgementDismiss, current.AcknowledgedBy)
	eventBus.mu.Lock()
	published := append([]*bus.Event(nil), eventBus.publishedEvents...)
	eventBus.mu.Unlock()
	require.NotEmpty(t, published, "dismissal publishes an ordinary task update")
}

func TestAcceptedManualPromptAcknowledgesTaskTurnError(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedTurnErrorTask(t, ctx, repo, "task-ack", "ws-ack")
	base := time.Now().UTC().Add(-time.Minute)
	stored, err := svc.RecordTaskTurnErrorAttention(ctx, "task-ack", models.TaskTurnError{
		StampValue: "stamp-a", SessionID: "session-a", OccurredAt: base, Message: "session a failed",
	})
	require.NoError(t, err)
	require.True(t, stored)

	// A prompt in another session cannot acknowledge an unseen sibling error.
	require.NoError(t, svc.AcknowledgeTaskTurnError(ctx, "task-ack", "session-b", "stamp-a"))
	task, err := repo.GetTask(ctx, "task-ack")
	require.NoError(t, err)
	current, _ := models.LoadTaskTurnError(task.Metadata)
	require.False(t, current.Acknowledged(), "a cross-session prompt must leave attention unchanged")

	// A stale observation does not clear a newer failure.
	require.NoError(t, svc.AcknowledgeTaskTurnError(ctx, "task-ack", "session-a", "stale-stamp"))
	task, _ = repo.GetTask(ctx, "task-ack")
	current, _ = models.LoadTaskTurnError(task.Metadata)
	require.False(t, current.Acknowledged())

	require.NoError(t, svc.AcknowledgeTaskTurnError(ctx, "task-ack", "session-a", "stamp-a"))
	task, _ = repo.GetTask(ctx, "task-ack")
	current, _ = models.LoadTaskTurnError(task.Metadata)
	require.True(t, current.Acknowledged())
	require.Equal(t, models.TurnErrorAcknowledgementPrompt, current.AcknowledgedBy)

	// A newer sibling failure replaces the pointer and is not acknowledged by
	// the older session's late input.
	_, err = svc.RecordTaskTurnErrorAttention(ctx, "task-ack", models.TaskTurnError{
		StampValue: "stamp-b", SessionID: "session-b", OccurredAt: base.Add(time.Second), Message: "session b failed",
	})
	require.NoError(t, err)
	require.NoError(t, svc.AcknowledgeTaskTurnError(ctx, "task-ack", "session-a", "stamp-a"))
	task, _ = repo.GetTask(ctx, "task-ack")
	current, _ = models.LoadTaskTurnError(task.Metadata)
	require.Equal(t, "stamp-b", current.StampValue)
	require.False(t, current.Acknowledged())
}

func TestDismissTaskTurnErrorRequiresStamp(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedTurnErrorTask(t, ctx, repo, "task-required", "ws-required")
	require.True(t, errors.Is(svc.DismissTaskTurnError(ctx, "task-required", ""), ErrTurnErrorStampRequired))
}

func TestRecordTaskTurnErrorAttentionIgnoresInvalidRecord(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedTurnErrorTask(t, ctx, repo, "task-invalid", "ws-invalid")
	stored, err := svc.RecordTaskTurnErrorAttention(ctx, "task-invalid", models.TaskTurnError{Message: ""})
	require.NoError(t, err)
	require.False(t, stored)
}
