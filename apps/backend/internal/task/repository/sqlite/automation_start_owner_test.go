package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAutomationStartOwnerSQLite(t *testing.T) {
	runAutomationStartOwnerContract(t, newRepoForEntityTests(t))
}

func TestAutomationStartOwnerPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	runAutomationStartOwnerContract(t, newPostgresMetadataCASRepo(t, database))
}

func runAutomationStartOwnerContract(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedMetadataCASTask(t, repo, map[string]interface{}{models.MetaKeyAutomationStartOwned: "original"})
	require.NoError(t, repo.MergeTaskMetadata(ctx, casTaskID, map[string]interface{}{models.MetaKeyAutomationStartOwned: "forged", "ordinary": "kept"}))
	task, err := repo.GetTask(ctx, casTaskID)
	require.NoError(t, err)
	require.Equal(t, "original", task.Metadata[models.MetaKeyAutomationStartOwned])
	require.Equal(t, "kept", task.Metadata["ordinary"])
	owned, err := repo.ListTasksWithAutomationStartOwned(ctx)
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.Equal(t, casTaskID, owned[0].ID)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, casTaskID, models.MetaKeyAutomationStartOwned, "successor"))
	removed, err := repo.RemoveTaskMetadataKeyIfValue(ctx, casTaskID, models.MetaKeyAutomationStartOwned, "original")
	require.NoError(t, err)
	require.False(t, removed)
	removed, err = repo.RemoveTaskMetadataKeyIfValue(ctx, casTaskID, models.MetaKeyAutomationStartOwned, "successor")
	require.NoError(t, err)
	require.True(t, removed)
	require.NoError(t, repo.MergeTaskMetadata(ctx, casTaskID, map[string]interface{}{models.MetaKeyAutomationStartOwned: "forged"}))
	task, err = repo.GetTask(ctx, casTaskID)
	require.NoError(t, err)
	require.False(t, models.AutomationStartOwned(task.Metadata))
}
