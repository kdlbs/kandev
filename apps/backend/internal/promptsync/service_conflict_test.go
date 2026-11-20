package promptsync

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

func (f *syncFixture) prompt(t *testing.T, name string) *promptmodels.Prompt {
	t.Helper()
	prompt, err := f.prompts.GetPromptByName(context.Background(), name)
	require.NoError(t, err)
	return prompt
}

func gitLabRequest(resolutions map[string]promptservice.ConflictChoice) *SetConfigRequest {
	return &SetConfigRequest{
		WorkspaceID: "ws-1", Provider: reposync.ProviderGitLab, ProjectPath: "acme/prompts", Resolutions: resolutions,
	}
}

func TestCheckListsConflictsWithoutWriting(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	_, err := f.prompts.CreatePrompt(ctx, "principles", "Mine")
	require.NoError(t, err)
	f.gitlab.files["principles.md"] = "Repo principles"
	f.gitlab.files["fresh.md"] = "New"

	result, err := f.svc.Check(ctx, gitLabRequest(nil))
	require.NoError(t, err)
	require.Len(t, result.Conflicts, 1)
	assert.Equal(t, Conflict{
		Name: "principles", Path: DefaultPath + "/principles.md",
		LocalContent: "Mine", RepositoryContent: "Repo principles",
	}, result.Conflicts[0])
	assert.Equal(t, promptmodels.PromptSourceManual, f.prompt(t, "principles").Source, "a check changes no prompt")
	_, err = f.prompts.GetPromptByName(ctx, "fresh")
	assert.ErrorIs(t, err, promptservice.ErrPromptNotFound, "a check creates no prompt")
	cfg, err := f.svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "acme/eng", cfg.ProjectPath, "a check stores no configuration")
}

func TestCheckStoredConfigListsUnresolvedConflicts(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	_, err := f.prompts.CreatePrompt(ctx, "principles", "Mine")
	require.NoError(t, err)
	f.gitlab.files["principles.md"] = "Repo principles"
	result, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"principles"}, result.Conflicts)

	check, err := f.svc.Check(ctx, nil)
	require.NoError(t, err)
	require.Len(t, check.Conflicts, 1)
	assert.Equal(t, "Mine", check.Conflicts[0].LocalContent)
}

func TestCheckFailureReturnsError(t *testing.T) {
	f := newSyncFixture(t)
	f.gitlab.listErr = errors.New("boom")
	_, err := f.svc.Check(context.Background(), gitLabRequest(nil))
	require.Error(t, err)
}

func TestSaveAndSyncAppliesResolutions(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	for _, name := range []string{"keep", "take"} {
		_, err := f.prompts.CreatePrompt(ctx, name, "Local "+name)
		require.NoError(t, err)
	}
	f.gitlab.files["keep.md"] = "Repo keep"
	f.gitlab.files["take.md"] = "Repo take"

	outcome, err := f.svc.SaveAndSync(ctx, gitLabRequest(map[string]promptservice.ConflictChoice{
		"keep": promptservice.ChoiceLocal, "take": promptservice.ChoiceRepository,
	}))
	require.NoError(t, err)
	require.NoError(t, outcome.SyncErr)
	assert.Equal(t, "acme/prompts", outcome.Config.ProjectPath)
	assert.Empty(t, outcome.Result.Conflicts)
	keep := f.prompt(t, "keep")
	assert.True(t, keep.IsOverride())
	assert.False(t, keep.Conflict)
	take := f.prompt(t, "take")
	assert.True(t, take.IsSynced())
	assert.Equal(t, "Repo take", take.Content)
	assert.Equal(t, "Local take", take.AlternateContent)
}

func TestSaveAndSyncKeepsConfigWhenSyncFails(t *testing.T) {
	f := newSyncFixture(t)
	f.gitlab.listErr = errors.New("boom")
	outcome, err := f.svc.SaveAndSync(context.Background(), gitLabRequest(nil))
	require.NoError(t, err)
	require.Error(t, outcome.SyncErr)
	assert.Equal(t, "acme/prompts", outcome.Config.ProjectPath)
}

func TestSyncWithResolutionsResolvesExistingConflict(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	_, err := f.prompts.CreatePrompt(ctx, "principles", "Mine")
	require.NoError(t, err)
	f.gitlab.files["principles.md"] = "Repo principles"
	_, err = f.svc.Sync(ctx)
	require.NoError(t, err)

	result, err := f.svc.SyncWithResolutions(ctx, map[string]promptservice.ConflictChoice{"principles": promptservice.ChoiceLocal})
	require.NoError(t, err)
	assert.Empty(t, result.Conflicts)
	assert.False(t, f.prompt(t, "principles").Conflict)
}

func TestNormalizeRejectsUnknownConflictChoice(t *testing.T) {
	err := gitLabRequest(map[string]promptservice.ConflictChoice{"x": "both"}).Normalize()
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestCheckSkipsPromptsIdenticalToTheirFile(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	_, err := f.prompts.CreatePrompt(ctx, "same", "Shared")
	require.NoError(t, err)
	f.gitlab.files["same.md"] = "Shared"
	result, err := f.svc.Check(ctx, gitLabRequest(nil))
	require.NoError(t, err)
	assert.Empty(t, result.Conflicts)
}

func TestOwnershipOnlySyncIsNotUnchanged(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	_, err := f.prompts.CreatePrompt(ctx, "principles", "Mine")
	require.NoError(t, err)
	f.gitlab.files["principles.md"] = "Repo"
	result, err := f.svc.SyncWithResolutions(ctx, map[string]promptservice.ConflictChoice{"principles": promptservice.ChoiceLocal})
	require.NoError(t, err)
	assert.Empty(t, result.Created)
	assert.Empty(t, result.Updated)
	assert.False(t, result.Unchanged, "adopting a prompt as an override is a change")
}
