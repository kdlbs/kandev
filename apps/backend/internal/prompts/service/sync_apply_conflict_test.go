package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
)

func promptNamed(t *testing.T, svc *Service, name string) *models.Prompt {
	t.Helper()
	prompt, err := svc.GetPromptByName(context.Background(), name)
	require.NoError(t, err)
	return prompt
}

func TestApplyAdoptsUnresolvedClashAsConflictedOverride(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	_, err := svc.CreatePrompt(ctx, "mine", "Manual text")
	require.NoError(t, err)

	result := applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "mine", Content: "Repo text", Path: "p/mine.md"},
		{Name: "code-review", Content: "Repo review", Path: "p/code-review.md"},
	}})
	assert.Empty(t, result.Created)
	assert.Empty(t, result.Warnings, "a conflict is not a warning")
	assert.Equal(t, []string{"code-review", "mine"}, result.Conflicts)
	mine := promptNamed(t, svc, "mine")
	assert.True(t, mine.IsOverride())
	assert.True(t, mine.Conflict)
	assert.Equal(t, "Manual text", mine.Content, "an undecided conflict keeps the local content")
	assert.Equal(t, "p/mine.md", mine.SourcePath)
	builtin := promptNamed(t, svc, "code-review")
	assert.True(t, builtin.Builtin)
	assert.True(t, builtin.Conflict, "built-in prompts are handled like manual prompts")
	assert.NotEqual(t, "Repo review", builtin.Content)

	result = applySynced(t, svc, SyncApplyInput{Desired: []SyncedPromptFile{
		{Name: "mine", Content: "Repo text", Path: "p/mine.md"},
		{Name: "code-review", Content: "Repo review", Path: "p/code-review.md"},
	}})
	assert.Equal(t, []string{"code-review", "mine"}, result.Conflicts, "a conflict stays reported until resolved")
}

func TestApplyResolvesClashesWithChoices(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	for _, name := range []string{"keep", "take"} {
		_, err := svc.CreatePrompt(ctx, name, "Local "+name)
		require.NoError(t, err)
	}
	files := []SyncedPromptFile{
		{Name: "keep", Content: "Repo keep", Path: "p/keep.md"},
		{Name: "take", Content: "Repo take", Path: "p/take.md"},
	}

	result := applySynced(t, svc, SyncApplyInput{Desired: files, Resolutions: map[string]ConflictChoice{
		"keep": ChoiceLocal, "take": ChoiceRepository,
	}})
	assert.Empty(t, result.Conflicts)
	keep := promptNamed(t, svc, "keep")
	assert.True(t, keep.IsOverride())
	assert.False(t, keep.Conflict)
	assert.Equal(t, "Local keep", keep.Content)
	take := promptNamed(t, svc, "take")
	assert.True(t, take.IsSynced())
	assert.Equal(t, "Repo take", take.Content)
	assert.Equal(t, "Local take", take.AlternateContent, "taking the repository version keeps the local one")

	overridden, err := svc.SavePromptVersions(ctx, take.ID, VersionSave{UseLocal: true})
	require.NoError(t, err)
	assert.Equal(t, "Local take", overridden.Content, "switching back restores the local version")
}

func TestApplyResolvesExistingConflictedOverride(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	for _, name := range []string{"keep", "take"} {
		_, err := svc.CreatePrompt(ctx, name, "Local "+name)
		require.NoError(t, err)
	}
	files := []SyncedPromptFile{
		{Name: "keep", Content: "Repo keep", Path: "p/keep.md"},
		{Name: "take", Content: "Repo take", Path: "p/take.md"},
	}
	applySynced(t, svc, SyncApplyInput{Desired: files})

	result := applySynced(t, svc, SyncApplyInput{Desired: files, Resolutions: map[string]ConflictChoice{
		"keep": ChoiceLocal, "take": ChoiceRepository,
	}})
	assert.Empty(t, result.Conflicts)
	assert.Equal(t, []string{"take"}, result.Updated)
	keep := promptNamed(t, svc, "keep")
	assert.False(t, keep.Conflict)
	assert.Equal(t, "Local keep", keep.Content)
	take := promptNamed(t, svc, "take")
	assert.True(t, take.IsSynced())
	assert.Equal(t, "Repo take", take.Content)
	assert.Equal(t, "Local take", take.AlternateContent)
}

func TestApplyVanishedFilesClearConflictsAndRestoreKeptContent(t *testing.T) {
	svc, cleanup := createService(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	for _, name := range []string{"flagged", "taken"} {
		_, err := svc.CreatePrompt(ctx, name, "Local "+name)
		require.NoError(t, err)
	}
	applySynced(t, svc, SyncApplyInput{
		Desired: []SyncedPromptFile{
			{Name: "flagged", Content: "Repo", Path: "p/flagged.md"},
			{Name: "taken", Content: "Repo", Path: "p/taken.md"},
			{Name: "code-review", Content: "Repo review", Path: "p/code-review.md"},
		},
		Resolutions: map[string]ConflictChoice{"taken": ChoiceRepository, "code-review": ChoiceRepository},
	})

	result := applySynced(t, svc, SyncApplyInput{})
	assert.Empty(t, result.Deleted)
	assert.Equal(t, []string{"code-review", "taken"}, result.Restored)
	flagged := promptNamed(t, svc, "flagged")
	assert.True(t, flagged.IsOverride())
	assert.False(t, flagged.Conflict, "a conflict ends when its file leaves the source")
	taken := promptNamed(t, svc, "taken")
	assert.Equal(t, models.PromptSourceManual, taken.Source)
	assert.Equal(t, "Local taken", taken.Content, "a kept local version is restored, not deleted")
	builtin := promptNamed(t, svc, "code-review")
	assert.True(t, builtin.Builtin, "sync never deletes a built-in prompt")
	assert.Equal(t, models.PromptSourceManual, builtin.Source)
	assert.NotEqual(t, "Repo review", builtin.Content)
}
