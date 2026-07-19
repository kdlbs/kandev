package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestReconcileTaskStateForRuntime_RejectsClarificationRace(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-runtime-race", "session-runtime-race", "")
	require.NoError(t, repo.UpdateTaskState(ctx, "task-runtime-race", v1.TaskStateReview))
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		"session-runtime-race",
		models.TaskSessionStateStarting,
		"",
	))

	taskRepo := newMockTaskRepo()
	called := false
	taskRepo.updateIfSessionState = func(
		ctx context.Context,
		taskID, sessionID string,
		expectedSessionState models.TaskSessionState,
		state v1.TaskState,
	) (bool, error) {
		called = true
		require.Equal(t, models.TaskSessionStateStarting, expectedSessionState)

		// Clarification wins after reconciliation's session read but before its
		// task write. The repository CAS must observe WAITING and reject the
		// stale STARTING -> IN_PROGRESS writer.
		require.NoError(t, repo.UpdateTaskSessionState(
			ctx,
			sessionID,
			models.TaskSessionStateWaitingForInput,
			"",
		))
		require.NoError(t, repo.UpdateTaskState(ctx, taskID, v1.TaskStateReview))
		_, updated, err := repo.UpdateTaskStateIfSessionState(
			ctx, taskID, sessionID, expectedSessionState, state,
		)
		return updated, err
	}

	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	require.NoError(t, svc.reconcileTaskStateForRuntime(
		ctx,
		"task-runtime-race",
		"session-runtime-race",
		v1.TaskStateInProgress,
	))
	require.True(t, called)

	task, err := repo.GetTask(ctx, "task-runtime-race")
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, task.State)
	session, err := repo.GetTaskSession(ctx, "session-runtime-race")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}
