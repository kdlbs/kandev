package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
)

func TestOverrideKeepsRepositoryVersionAndRevertUsesItAtOnce(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	synced, err := repo.CreateSyncedPrompt(ctx, "review", "Repo", "p/review.md")
	require.NoError(t, err)

	require.True(t, overrideForTest(t, repo, synced.ID))
	override := mustGet(t, repo, synced.ID)
	assert.Equal(t, "Repo", override.AlternateContent, "an override keeps the repository version")
	override.Content = "Mine"
	require.NoError(t, repo.UpdatePrompt(ctx, override))
	assert.Equal(t, "Repo", mustGet(t, repo, synced.ID).AlternateContent, "a local edit leaves it")

	changed, err := repo.RevertOverride(ctx, synced.ID)
	require.NoError(t, err)
	require.True(t, changed)
	reverted := mustGet(t, repo, synced.ID)
	assert.Equal(t, "Repo", reverted.Content, "reverting uses the stored repository version at once")
	assert.Equal(t, "Mine", reverted.AlternateContent)
}

func TestOverrideRepositoryVersionFollowsTheFile(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	synced, err := repo.CreateSyncedPrompt(ctx, "review", "Repo v1", "p/review.md")
	require.NoError(t, err)
	require.True(t, overrideForTest(t, repo, synced.ID))

	changed, err := repo.SetOverrideRepositoryContent(ctx, synced.ID, "Repo v2")
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = repo.SetOverrideRepositoryContent(ctx, synced.ID, "Repo v2")
	require.NoError(t, err)
	assert.False(t, changed, "an unchanged file writes nothing")
	assert.Equal(t, "Repo v2", mustGet(t, repo, synced.ID).AlternateContent)

	manual := &models.Prompt{Name: "manual", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))
	changed, err = repo.SetOverrideRepositoryContent(ctx, manual.ID, "Repo")
	require.NoError(t, err)
	assert.False(t, changed, "only an override takes a repository version")
}

func TestDetachOverrideDropsConflictAndRepositoryVersion(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	manual := &models.Prompt{Name: "clash", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))
	_, err := repo.AdoptPromptAsOverride(ctx, manual, "p/clash.md", "Repo", true)
	require.NoError(t, err)

	changed, err := repo.DetachOverride(ctx, manual.ID)
	require.NoError(t, err)
	assert.True(t, changed)
	detached := mustGet(t, repo, manual.ID)
	assert.True(t, detached.IsOverride())
	assert.False(t, detached.Conflict)
	assert.Empty(t, detached.AlternateContent)
	assert.Equal(t, "Mine", detached.Content)
}

func TestReleaseCanSwitchAnOverrideToItsRepositoryVersion(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	manual := &models.Prompt{Name: "clash", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))
	_, err := repo.AdoptPromptAsOverride(ctx, manual, "p/clash.md", "Repo", false)
	require.NoError(t, err)

	_, err = repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{UseOther: []string{"clash"}})
	require.NoError(t, err)
	released := mustGet(t, repo, manual.ID)
	assert.Equal(t, models.PromptSourceManual, released.Source)
	assert.Equal(t, "Repo", released.Content)
}
