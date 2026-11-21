package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
)

func TestAdoptionSkipsAPromptRenamedOrEditedAfterTheSyncReadIt(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	renamed := &models.Prompt{Name: "review", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, renamed))
	edited := &models.Prompt{Name: "notes", Content: "Mine"}
	require.NoError(t, repo.CreatePrompt(ctx, edited))
	seenRenamed, seenEdited := *renamed, *edited
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE custom_prompts SET name = ? WHERE id = ?`), "renamed", renamed.ID)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE custom_prompts SET content = ? WHERE id = ?`), "Edited", edited.ID)
	require.NoError(t, err)

	adopted, err := repo.AdoptPromptAsOverride(ctx, &seenRenamed, "p/review.md", "Repo", true)
	require.NoError(t, err)
	assert.False(t, adopted, "a renamed prompt is not adopted for the old name's file")
	adopted, err = repo.AdoptPromptAsSynced(ctx, &seenEdited, "Mine", "p/notes.md")
	require.NoError(t, err)
	assert.False(t, adopted, "an edited prompt is not adopted on the strength of its old text")
	assert.Equal(t, models.PromptSourceManual, mustGet(t, repo, renamed.ID).Source)
	assert.Equal(t, "Edited", mustGet(t, repo, edited.ID).Content)
}

func TestReleaseDeletesOnlyTheReviewedRepositoryOnlyPrompts(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	reviewed, err := repo.CreateSyncedPrompt(ctx, "reviewed", "Repo reviewed", "p/reviewed.md")
	require.NoError(t, err)
	late, err := repo.CreateSyncedPrompt(ctx, "late", "Repo late", "p/late.md")
	require.NoError(t, err)

	_, err = repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{DeleteRepositoryOnly: []string{"reviewed"}})
	require.NoError(t, err)
	_, err = repo.GetPromptByID(ctx, reviewed.ID)
	assert.Error(t, err, "the reviewed prompt is deleted")
	kept := mustGet(t, repo, late.ID)
	assert.Equal(t, models.PromptSourceManual, kept.Source, "a prompt synced after the review is released, not deleted")
}

func TestReleaseAdvancesTheRevisionOfReleasedPrompts(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	prompt, err := repo.CreateSyncedPrompt(ctx, "hook", "Repo", "p/hook.md")
	require.NoError(t, err)
	old := time.Now().UTC().Add(-time.Hour)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE custom_prompts SET updated_at = ? WHERE id = ?`), old, prompt.ID)
	require.NoError(t, err)

	_, err = repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{})
	require.NoError(t, err)
	assert.True(t, mustGet(t, repo, prompt.ID).UpdatedAt.After(old), "a save based on the pre-release revision is rejected")
}
