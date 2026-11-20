package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
)

func TestOverridePromptsAreLocallyWritable(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	synced, err := repo.CreateSyncedPrompt(ctx, "post-pr-review", "Default", "p/post-pr-review.md")
	require.NoError(t, err)

	changed, err := repo.RevertOverride(ctx, synced.ID)
	require.NoError(t, err)
	assert.False(t, changed, "the from-source guard must match")
	assert.True(t, overrideForTest(t, repo, synced.ID))

	override, err := repo.GetPromptByID(ctx, synced.ID)
	require.NoError(t, err)
	assert.True(t, override.IsOverride())
	assert.Equal(t, "p/post-pr-review.md", override.SourcePath)

	override.Content = "Mine"
	require.NoError(t, repo.UpdatePrompt(ctx, override))
	agent, err := repo.GetPromptByID(ctx, synced.ID)
	require.NoError(t, err)
	agent.Content = "Agent"
	require.NoError(t, repo.UpdatePromptForAgent(ctx, agent, agent.Name))

	updated, err := repo.UpdateSyncedPrompt(ctx, synced.ID, "Repo", "p/post-pr-review.md")
	require.NoError(t, err)
	assert.False(t, updated, "sync must never write an override")

	released, err := repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), released, "removing sync releases overrides too")
	manual, err := repo.GetPromptByID(ctx, synced.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PromptSourceManual, manual.Source)
}

func TestDeleteOverridePrompt(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	synced, err := repo.CreateSyncedPrompt(ctx, "gone", "x", "p/gone.md")
	require.NoError(t, err)
	require.True(t, overrideForTest(t, repo, synced.ID))
	require.NoError(t, repo.DeletePrompt(ctx, synced.ID))
	_, err = repo.GetPromptByID(ctx, synced.ID)
	assert.Error(t, err)
}
