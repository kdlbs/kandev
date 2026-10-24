package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestQueueRecoverySessionDiscoveryIsBounded(t *testing.T) {
	repo := newRepoForSessionTests(t)
	checkQueueRecoveryDiscovery(t, repo)
}

func TestQueueRecoverySessionDiscoveryPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	checkQueueRecoveryDiscovery(t, repo)
}

func checkQueueRecoveryDiscovery(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	missing, err := repo.ListQueueRecoverySessions(ctx, "missing-task")
	require.NoError(t, err)
	require.Empty(t, missing)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Recovery"}))
	for i := range 256 {
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: fmt.Sprintf("session-%03d", i), TaskID: "task", Metadata: map[string]interface{}{"private": "secret"}, WorkspacePath: "/private/path"}))
	}
	sessions, err := repo.ListQueueRecoverySessions(ctx, "task")
	require.NoError(t, err)
	require.Len(t, sessions, 256)
	for _, session := range sessions {
		require.Empty(t, session.Metadata)
		require.Empty(t, session.WorkspacePath)
		require.NotEmpty(t, session.QueueIncarnationID)
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "extra", TaskID: "task"}))
	oversize, err := repo.ListQueueRecoverySessions(ctx, "task")
	require.ErrorIs(t, err, messagequeue.ErrRecoveryLimit)
	require.Nil(t, oversize)
}
