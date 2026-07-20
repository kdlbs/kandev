package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRestoreTaskMessageRollback_RejectionReturnsPersistedTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		sessionID,
		models.TaskSessionStateWaitingForInput,
		"",
	))

	before, err := repo.GetTask(ctx, "task-123")
	require.NoError(t, err)
	returned, updated, err := svc.RestoreTaskMessageRollback(
		ctx,
		before.ID,
		sessionID,
		models.TaskSessionStateRunning,
		v1.TaskStateReview,
		"restored-step",
	)
	require.NoError(t, err)
	require.False(t, updated)
	require.Equal(t, before.State, returned.State)
	require.Equal(t, before.WorkflowStepID, returned.WorkflowStepID)
	require.Equal(t, before.UpdatedAt, returned.UpdatedAt)

	persisted, err := repo.GetTask(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before.State, persisted.State)
	require.Equal(t, before.WorkflowStepID, persisted.WorkflowStepID)
	require.Empty(t, eventBus.GetPublishedEvents())
}
