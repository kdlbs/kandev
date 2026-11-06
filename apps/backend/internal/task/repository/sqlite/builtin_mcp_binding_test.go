package sqlite

import (
	"context"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestBuiltinBindingPersistenceFencesExecution(t *testing.T) {
	testBuiltinBindingPersistence(t, newRepoForSessionTests(t))
}
func TestPostgresBuiltinBindingPersistenceFencesExecution(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	testBuiltinBindingPersistence(t, repo)
}
func testBuiltinBindingPersistence(t *testing.T, repo *Repository) {
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-binding")
	seedExecutorRunningCleanupSession(t, repo, "task-binding", "session-binding", models.TaskSessionStateRunning, "successor")
	row, err := repo.GetExecutorRunningBySessionID(ctx, "session-binding")
	require.NoError(t, err)
	row.Metadata = map[string]any{"context_window": 123, p.BuiltinBindingMetadataKey: "current"}
	row.ResumeToken = "resume"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	stale := *row
	require.ErrorIs(t, repo.UpdateBuiltinMCPBinding(ctx, "session-binding", "predecessor", "stale"), models.ErrExecutionRotated)
	value := map[string]any{"update_incomplete": true}
	require.NoError(t, repo.UpdateBuiltinMCPBinding(ctx, "session-binding", "successor", value))
	row, err = repo.GetExecutorRunningBySessionID(ctx, "session-binding")
	require.NoError(t, err)
	require.Equal(t, value, row.Metadata[p.BuiltinBindingMetadataKey])
	require.EqualValues(t, 123, row.Metadata["context_window"])
	require.Equal(t, "resume", row.ResumeToken)
	require.NoError(t, repo.UpdateBuiltinMCPBinding(ctx, "session-binding", "successor", "sealed binding"))
	row, err = repo.GetExecutorRunningBySessionID(ctx, "session-binding")
	require.NoError(t, err)
	require.Equal(t, "sealed binding", row.Metadata[p.BuiltinBindingMetadataKey])
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &stale))
	row, err = repo.GetExecutorRunningBySessionID(ctx, "session-binding")
	require.NoError(t, err)
	require.Equal(t, "sealed binding", row.Metadata[p.BuiltinBindingMetadataKey], "a delayed status snapshot cannot overwrite a binding update")
	require.Error(t, repo.UpdateBuiltinMCPBinding(ctx, "session-binding", "", "invalid"))
}
