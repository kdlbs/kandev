package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
)

func mustGet(t *testing.T, repo *sqliteRepository, id string) *models.Prompt {
	t.Helper()
	prompt, err := repo.GetPromptByID(context.Background(), id)
	require.NoError(t, err)
	return prompt
}

func TestRevertKeepsAlternateContentAndOverrideRestoresIt(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	synced, err := repo.CreateSyncedPrompt(ctx, "review", "Repo v1", "p/review.md")
	require.NoError(t, err)

	require.True(t, overrideForTest(t, repo, synced.ID))
	override := mustGet(t, repo, synced.ID)
	assert.Equal(t, "Repo v1", override.Content, "a first override starts from the synced content")
	override.Content = "Mine"
	require.NoError(t, repo.UpdatePrompt(ctx, override))

	changed, err := repo.RevertOverride(ctx, synced.ID)
	require.NoError(t, err)
	require.True(t, changed)
	reverted := mustGet(t, repo, synced.ID)
	assert.True(t, reverted.IsSynced())
	assert.Equal(t, "Mine", reverted.AlternateContent, "reverting keeps the local version")

	updated, err := repo.UpdateSyncedPrompt(ctx, synced.ID, "Repo v2", "p/review.md")
	require.NoError(t, err)
	require.True(t, updated)
	assert.Equal(t, "Mine", mustGet(t, repo, synced.ID).AlternateContent, "sync updates preserve the kept copy")

	require.True(t, overrideForTest(t, repo, synced.ID))
	restored := mustGet(t, repo, synced.ID)
	assert.True(t, restored.IsOverride())
	assert.Equal(t, "Mine", restored.Content, "turning the override on again restores the local version")
	assert.Equal(t, "Repo v2", restored.AlternateContent, "the repository version becomes the other version")
}

func TestOwnershipTransitionsAreGuarded(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	manual := &models.Prompt{Name: "manual", Content: "Hand written"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))

	for name, transition := range map[string]func() (bool, error){
		"revert":  func() (bool, error) { return repo.RevertOverride(ctx, manual.ID) },
		"clear":   func() (bool, error) { return repo.ClearOverrideConflict(ctx, manual.ID) },
		"restore": func() (bool, error) { return repo.RestoreSyncedPrompt(ctx, manual.ID) },
	} {
		changed, err := transition()
		require.NoError(t, err, name)
		assert.False(t, changed, "%s must not touch a manual prompt", name)
	}
	assert.Equal(t, models.PromptSourceManual, mustGet(t, repo, manual.ID).Source)
}

func TestAdoptManualPromptAsOverrideWithConflict(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	manual := &models.Prompt{Name: "principles", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))

	changed, err := repo.AdoptPromptAsOverride(ctx, manual, "p/principles.md", "Repo", true)
	require.NoError(t, err)
	require.True(t, changed)
	adopted := mustGet(t, repo, manual.ID)
	assert.True(t, adopted.IsOverride())
	assert.True(t, adopted.Conflict)
	assert.Equal(t, "Mine", adopted.Content)
	assert.Equal(t, "p/principles.md", adopted.SourcePath)

	changed, err = repo.AdoptPromptAsOverride(ctx, manual, "p/principles.md", "Repo", false)
	require.NoError(t, err)
	assert.False(t, changed, "only a manual prompt can be adopted")

	adopted.Content = "Edited"
	require.NoError(t, repo.UpdatePromptForAgent(ctx, adopted, adopted.Name))
	assert.True(t, mustGet(t, repo, manual.ID).Conflict, "a local edit does not resolve the conflict")

	changed, err = repo.ClearOverrideConflict(ctx, manual.ID)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.False(t, mustGet(t, repo, manual.ID).Conflict)

	_, err = repo.AdoptPromptAsOverride(ctx, manual, "p/principles.md", "Repo", false)
	require.NoError(t, err)
	changed, err = repo.RevertOverride(ctx, manual.ID)
	require.NoError(t, err)
	require.True(t, changed)
	reverted := mustGet(t, repo, manual.ID)
	assert.False(t, reverted.Conflict, "reverting resolves the conflict")
	assert.Equal(t, "Edited", reverted.AlternateContent)
}

func TestAdoptManualPromptAsSyncedKeepsAlternateContent(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	manual := &models.Prompt{Name: "principles", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, manual))

	changed, err := repo.AdoptPromptAsSynced(ctx, manual, "Repo", "p/principles.md")
	require.NoError(t, err)
	require.True(t, changed)
	adopted := mustGet(t, repo, manual.ID)
	assert.True(t, adopted.IsSynced())
	assert.Equal(t, "Repo", adopted.Content)
	assert.Equal(t, "Mine", adopted.AlternateContent)
	assert.False(t, adopted.Conflict)

	changed, err = repo.RestoreSyncedPrompt(ctx, manual.ID)
	require.NoError(t, err)
	require.True(t, changed)
	restored := mustGet(t, repo, manual.ID)
	assert.Equal(t, models.PromptSourceManual, restored.Source)
	assert.Equal(t, "Mine", restored.Content, "restoring returns the kept local version")
	assert.Empty(t, restored.AlternateContent)
	assert.Empty(t, restored.SourcePath)
}

func TestSyncNeverDeletesBuiltinPrompt(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	builtin, err := repo.GetPromptByName(ctx, "code-review")
	require.NoError(t, err)
	require.True(t, builtin.Builtin)
	_, err = repo.AdoptPromptAsSynced(ctx, builtin, "Repo review", "p/code-review.md")
	require.NoError(t, err)

	deleted, err := repo.DeleteSyncedPrompt(ctx, builtin.ID)
	require.NoError(t, err)
	assert.False(t, deleted)
	assert.True(t, mustGet(t, repo, builtin.ID).Builtin)
}

func TestReleaseClearsConflictAndKeptContent(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	first := &models.Prompt{Name: "first", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, first))
	second := &models.Prompt{Name: "second", Content: "Also mine"}
	require.NoError(t, repo.CreatePrompt(ctx, second))
	_, err := repo.AdoptPromptAsOverride(ctx, first, "p/first.md", "Repo first", true)
	require.NoError(t, err)
	_, err = repo.AdoptPromptAsSynced(ctx, second, "Repo", "p/second.md")
	require.NoError(t, err)

	released, err := repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), released)
	for _, id := range []string{first.ID, second.ID} {
		prompt := mustGet(t, repo, id)
		assert.Equal(t, models.PromptSourceManual, prompt.Source)
		assert.False(t, prompt.Conflict)
		assert.Empty(t, prompt.AlternateContent)
	}
	assert.Equal(t, "Repo", mustGet(t, repo, second.ID).Content, "release keeps the current content")
}

func TestReleaseRestoresChosenKeptVersions(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	ids := map[string]string{}
	for _, name := range []string{"restore-me", "keep-repo"} {
		prompt := &models.Prompt{Name: name, Content: "Local " + name}
		require.NoError(t, repo.CreatePrompt(ctx, prompt))
		_, err := repo.AdoptPromptAsSynced(ctx, prompt, "Repo "+name, "p/"+name+".md")
		require.NoError(t, err)
		ids[name] = prompt.ID
	}

	released, err := repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{UseOther: []string{"restore-me", "missing"}})
	require.NoError(t, err)
	assert.Equal(t, int64(2), released)
	restored := mustGet(t, repo, ids["restore-me"])
	assert.Equal(t, "Local restore-me", restored.Content, "a chosen prompt gets its kept version back")
	assert.Equal(t, models.PromptSourceManual, restored.Source)
	kept := mustGet(t, repo, ids["keep-repo"])
	assert.Equal(t, "Repo keep-repo", kept.Content)
	assert.Empty(t, kept.AlternateContent)
}
