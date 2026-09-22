package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// TestPostgresCoordinatorHandoffPrimarySwap exercises the task-row lock and
// portable predicates on the second supported SQL dialect. It skips unless
// KANDEV_TEST_POSTGRES_DSN is configured.
func TestPostgresCoordinatorHandoffPrimarySwap(t *testing.T) {
	repo := openPostgresRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "handoff-pg-task", Title: "Coordinator"}))
	for _, session := range []*models.TaskSession{
		{ID: "handoff-pg-old", TaskID: "handoff-pg-task", QueueIncarnationID: "pg-old-inc", State: models.TaskSessionStateRunning},
		{ID: "handoff-pg-new", TaskID: "handoff-pg-task", QueueIncarnationID: "pg-new-inc", State: models.TaskSessionStateWaitingForInput},
	} {
		require.NoError(t, repo.CreateTaskSession(ctx, session))
	}
	require.NoError(t, repo.SetSessionPrimary(ctx, "handoff-pg-old"))

	changed, err := repo.PromoteCoordinatorSuccessor(ctx, "handoff-pg-task",
		"handoff-pg-old", "handoff-pg-new", "pg-old-inc", "pg-new-inc", "pg-operation")
	require.NoError(t, err)
	require.True(t, changed)
	primary, err := repo.GetPrimarySessionByTaskID(ctx, "handoff-pg-task")
	require.NoError(t, err)
	require.Equal(t, "handoff-pg-new", primary.ID)
}
