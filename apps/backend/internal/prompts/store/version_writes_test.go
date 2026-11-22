package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
)

// overrideForTest turns a synced prompt into an override the way the
// operator's save does: the kept local version (or the synced content) is
// used and the repository version is kept as the other version.
func overrideForTest(t *testing.T, repo *sqliteRepository, id string) bool {
	t.Helper()
	prompt, err := repo.GetPromptByID(context.Background(), id)
	require.NoError(t, err)
	local := prompt.AlternateContent
	if local == "" {
		local = prompt.Content
	}
	changed, err := repo.SavePromptVersions(context.Background(), id, VersionsOf(prompt), PromptVersions{
		Source: models.PromptSourceOverride, Content: local, AlternateContent: prompt.Content,
	})
	require.NoError(t, err)
	return changed
}

func TestSavePromptVersionsWritesBothVersionsAtomically(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	synced, err := repo.CreateSyncedPrompt(ctx, "hook", "Repo", "p/hook.md")
	require.NoError(t, err)

	changed, err := repo.SavePromptVersions(ctx, synced.ID, VersionsOf(synced), PromptVersions{
		Source: models.PromptSourceOverride, Content: "Mine", AlternateContent: "Repo",
	})
	require.NoError(t, err)
	assert.True(t, changed)
	saved, err := repo.GetPromptByID(ctx, synced.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PromptSourceOverride, saved.Source)
	assert.Equal(t, "Mine", saved.Content)
	assert.Equal(t, "Repo", saved.AlternateContent)
	assert.Equal(t, "p/hook.md", saved.SourcePath)

	changed, err = repo.SavePromptVersions(ctx, synced.ID, VersionsOf(synced), PromptVersions{
		Source: models.PromptSourceSync, Content: "Repo", AlternateContent: "Stale",
	})
	require.NoError(t, err)
	assert.False(t, changed, "a save from an obsolete pre-image must not write")
	unchanged, err := repo.GetPromptByID(ctx, synced.ID)
	require.NoError(t, err)
	assert.Equal(t, "Mine", unchanged.Content)
}

func TestSavePromptVersionsClearsConflictAndNeverWritesManualPrompts(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	manual := &models.Prompt{Name: "clash", Content: "Local"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))

	changed, err := repo.SavePromptVersions(ctx, manual.ID, VersionsOf(manual), PromptVersions{
		Source: models.PromptSourceOverride, Content: "Local", AlternateContent: "Repo",
	})
	require.NoError(t, err)
	assert.False(t, changed, "a manual prompt has no repository version")

	_, err = repo.AdoptPromptAsOverride(ctx, manual, "p/clash.md", "Repo", true)
	require.NoError(t, err)
	adopted, err := repo.GetPromptByID(ctx, manual.ID)
	require.NoError(t, err)
	require.True(t, adopted.Conflict)
	changed, err = repo.SavePromptVersions(ctx, manual.ID, VersionsOf(adopted), PromptVersions{
		Source: models.PromptSourceSync, Content: "Repo", AlternateContent: "Local",
	})
	require.NoError(t, err)
	assert.True(t, changed)
	resolved, err := repo.GetPromptByID(ctx, manual.ID)
	require.NoError(t, err)
	assert.False(t, resolved.Conflict, "saving a version is the user's choice")
	assert.True(t, resolved.IsSynced())
}

func TestReleaseCanDeleteRepositoryOnlyPrompts(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	only, err := repo.CreateSyncedPrompt(ctx, "only", "Repo only", "p/only.md")
	require.NoError(t, err)
	kept := &models.Prompt{Name: "kept", Content: "Local kept"}
	require.NoError(t, repo.CreatePrompt(ctx, kept))
	_, err = repo.AdoptPromptAsSynced(ctx, kept, "Repo kept", "p/kept.md")
	require.NoError(t, err)
	over, err := repo.CreateSyncedPrompt(ctx, "over", "Repo over", "p/over.md")
	require.NoError(t, err)
	require.True(t, overrideForTest(t, repo, over.ID))
	manual := &models.Prompt{Name: "manual", Content: "Manual"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))

	changed, err := repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{DeleteRepositoryOnly: []string{"only", "code-review"}})
	require.NoError(t, err)
	assert.Equal(t, int64(3), changed, "one deleted and two released")
	_, err = repo.GetPromptByID(ctx, only.ID)
	assert.Error(t, err, "a prompt that only came from the repository is deleted")
	for _, id := range []string{kept.ID, over.ID, manual.ID} {
		prompt, err := repo.GetPromptByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, models.PromptSourceManual, prompt.Source)
	}
}

func TestReleaseNeverDeletesBuiltinPrompts(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	builtin, err := repo.GetPromptByName(ctx, "code-review")
	require.NoError(t, err)
	require.True(t, builtin.Builtin)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE custom_prompts SET source = ?, source_path = ?, alternate_content = '' WHERE id = ?`),
		models.PromptSourceSync, "p/code-review.md", builtin.ID)
	require.NoError(t, err)

	_, err = repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{DeleteRepositoryOnly: []string{"only", "code-review"}})
	require.NoError(t, err)
	released, err := repo.GetPromptByID(ctx, builtin.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PromptSourceManual, released.Source)
}
