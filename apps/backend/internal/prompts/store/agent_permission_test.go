package store

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/prompts/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPromptAgentPermissionMigration(t *testing.T) {
	database := createUnseededPromptDB(t)
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err := database.Exec(`INSERT INTO custom_prompts (id,name,content,builtin,created_at,updated_at) VALUES (?,?,?,0,?,?)`, "old", "human", "Keep me", stamp, stamp)
	require.NoError(t, err)
	for range 2 {
		repo, err := newSQLiteRepositoryWithDB(database, database)
		require.NoError(t, err)
		exists, err := db.ColumnExists(database, "custom_prompts", "allow_agent_edits")
		require.NoError(t, err)
		require.True(t, exists, "upgrade must add agent-edit permission")
		var allowed int
		require.NoError(t, database.Get(&allowed, `SELECT allow_agent_edits FROM custom_prompts WHERE id = 'old'`))
		require.Zero(t, allowed)
		prompt, err := repo.GetPromptByName(context.Background(), "human")
		require.NoError(t, err)
		require.Equal(t, "Keep me", prompt.Content)
		require.True(t, stamp.Equal(prompt.UpdatedAt))
	}
}

func TestPromptAgentWriteGuards(t *testing.T) {
	repo, cleanup := createTestRepo(t)
	t.Cleanup(cleanup)
	assertAgentWriteGuards(t, repo)
	assertOperatorWriteGuard(t, repo)
}

func TestPostgresPromptAgentWriteGuards(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(database, database)
	require.NoError(t, err)
	assertAgentWriteGuards(t, repo)
	assertOperatorWriteGuard(t, repo)
	assertPermissionReplay(t, database)
}

func assertPermissionReplay(t *testing.T, database *sqlx.DB) {
	t.Helper()
	for range 2 {
		_, err := newSQLiteRepositoryWithDB(database, database)
		require.NoError(t, err)
	}
}

func assertAgentWriteGuards(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	for _, mutation := range []string{"revoke", "rename", "content", "delete", "builtin", "allowed"} {
		t.Run(mutation, func(t *testing.T) {
			prompt := &models.Prompt{Name: mutation, Content: "Original", AllowAgentEdits: true, Builtin: mutation == "builtin"}
			require.NoError(t, repo.CreatePrompt(ctx, prompt))
			observed, err := repo.GetPromptByName(ctx, mutation)
			require.NoError(t, err)
			switch mutation {
			case "revoke":
				prompt.AllowAgentEdits = false
			case "rename":
				prompt.Name = "renamed"
			case "content":
				prompt.Content = "Human revision"
			}
			if mutation == "delete" {
				require.NoError(t, repo.DeletePrompt(ctx, prompt.ID))
			} else if mutation != "allowed" && mutation != "builtin" {
				require.NoError(t, repo.UpdatePrompt(ctx, prompt))
			}
			observed.Content = "Agent revision"
			err = repo.UpdatePromptForAgent(ctx, observed, mutation)
			if mutation == "allowed" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrPromptWriteRejected)
			}
			if mutation == "delete" {
				return
			}
			saved, err := repo.GetPromptByID(ctx, prompt.ID)
			require.NoError(t, err)
			if mutation == "allowed" {
				require.Equal(t, "Agent revision", saved.Content)
			} else {
				require.Equal(t, prompt.Content, saved.Content)
			}
			if mutation == "revoke" || mutation == "builtin" {
				require.False(t, saved.AllowAgentEdits)
			}
		})
	}
}

func assertOperatorWriteGuard(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	prompt := &models.Prompt{Name: "operator-race", Content: "Original", AllowAgentEdits: true}
	require.NoError(t, repo.CreatePrompt(ctx, prompt))
	stale, err := repo.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	prompt.AllowAgentEdits = false
	require.NoError(t, repo.UpdatePrompt(ctx, prompt))
	revoked, err := repo.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	stale.Content = "Stale content save"
	require.ErrorIs(t, repo.UpdatePrompt(ctx, stale), ErrPromptWriteRejected)
	saved, err := repo.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	require.False(t, saved.AllowAgentEdits)
	require.Equal(t, "Original", saved.Content)
	require.True(t, revoked.UpdatedAt.Equal(saved.UpdatedAt))
}
