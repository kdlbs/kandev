package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/prompts/models"
)

func TestPromptSourceColumnsMigrateLegacyRowsAsManual(t *testing.T) {
	database := createUnseededPromptDB(t)
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err := database.Exec(`INSERT INTO custom_prompts (id,name,content,builtin,created_at,updated_at) VALUES ('legacy','legacy','Keep',0,?,?)`, stamp, stamp)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = newSQLiteRepositoryWithDB(database, database)
		require.NoError(t, err)
	}
	repo, err := newSQLiteRepositoryWithDB(database, database)
	require.NoError(t, err)
	prompt, err := repo.GetPromptByID(context.Background(), "legacy")
	require.NoError(t, err)
	assert.Equal(t, models.PromptSourceManual, prompt.Source)
	assert.Empty(t, prompt.SourcePath)
	assert.Equal(t, "Keep", prompt.Content)
	assert.True(t, stamp.Equal(prompt.UpdatedAt))
}

func TestSyncedPromptWritesAreOwnershipGuarded(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()

	synced, err := repo.CreateSyncedPrompt(ctx, "principles", "Be simple", ".kandev/prompts/principles.md")
	require.NoError(t, err)
	assert.Equal(t, models.PromptSourceSync, synced.Source)

	read, err := repo.GetPromptByName(ctx, "principles")
	require.NoError(t, err)
	assert.Equal(t, models.PromptSourceSync, read.Source)
	assert.Equal(t, ".kandev/prompts/principles.md", read.SourcePath)

	operator := *read
	operator.Content = "operator edit"
	assert.True(t, errors.Is(repo.UpdatePrompt(ctx, &operator), ErrPromptWriteRejected))
	agent := *read
	agent.Content = "agent edit"
	assert.True(t, errors.Is(repo.UpdatePromptForAgent(ctx, &agent, read.Name), ErrPromptWriteRejected))
	require.NoError(t, repo.DeletePrompt(ctx, read.ID))

	still, err := repo.GetPromptByID(ctx, read.ID)
	require.NoError(t, err)
	assert.Equal(t, "Be simple", still.Content)

	changed, err := repo.UpdateSyncedPrompt(ctx, read.ID, "Be simpler", ".kandev/prompts/principles.md")
	require.NoError(t, err)
	assert.True(t, changed)

	released, err := repo.ReleaseSyncedPrompts(ctx, ReleaseOptions{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), released)
	manual, err := repo.GetPromptByID(ctx, read.ID)
	require.NoError(t, err)
	assert.Equal(t, models.PromptSourceManual, manual.Source)
	assert.Empty(t, manual.SourcePath)
	assert.Equal(t, "Be simpler", manual.Content)

	changed, err = repo.UpdateSyncedPrompt(ctx, read.ID, "sync over manual", "x.md")
	require.NoError(t, err)
	assert.False(t, changed, "sync must never overwrite a manual prompt")
	deleted, err := repo.DeleteSyncedPrompt(ctx, read.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "sync must never delete a manual prompt")
}

func TestDeleteSyncedPromptRemovesOnlySyncedRows(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	synced, err := repo.CreateSyncedPrompt(ctx, "gone", "Bye", "gone.md")
	require.NoError(t, err)
	deleted, err := repo.DeleteSyncedPrompt(ctx, synced.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	_, err = repo.GetPromptByID(ctx, synced.ID)
	assert.Error(t, err)
}
