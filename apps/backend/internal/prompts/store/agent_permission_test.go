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

func TestPromptFreshSchemaOmitsAgentPermission(t *testing.T) {
	database := createUnseededPromptDB(t)
	defer func() { require.NoError(t, database.Close()) }()
	repo, err := newSQLiteRepositoryWithDB(database, database)
	require.NoError(t, err)
	exists, err := db.ColumnExists(database, "custom_prompts", "allow_agent_edits")
	require.NoError(t, err)
	require.False(t, exists)
	_, err = newSQLiteRepositoryWithDB(database, database)
	require.NoError(t, err)
	_, err = repo.GetPromptByName(context.Background(), "code-review")
	require.NoError(t, err)
}

func TestPromptAgentWriteIgnoresHistoricalPermission(t *testing.T) {
	database := createUnseededPromptDB(t)
	defer func() { require.NoError(t, database.Close()) }()
	_, err := database.Exec(`ALTER TABLE custom_prompts ADD COLUMN allow_agent_edits INTEGER NOT NULL DEFAULT 0`)
	require.NoError(t, err)
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, allowed := range []int{0, 1} {
		id := "legacy-zero"
		if allowed == 1 {
			id = "legacy-one"
		}
		_, err := database.Exec(`INSERT INTO custom_prompts (id,name,content,builtin,allow_agent_edits,created_at,updated_at) VALUES (?,?,?,0,?,?,?)`, id, id, "Keep me", allowed, stamp, stamp)
		require.NoError(t, err)
	}
	repo, err := newSQLiteRepositoryWithDB(database, database)
	require.NoError(t, err)
	for _, id := range []string{"legacy-zero", "legacy-one"} {
		t.Run(id, func(t *testing.T) {
			prompt, err := repo.GetPromptByID(context.Background(), id)
			require.NoError(t, err)
			require.Equal(t, "Keep me", prompt.Content)
			require.True(t, stamp.Equal(prompt.UpdatedAt))
			prompt.Content = "Agent revision"
			require.NoError(t, repo.UpdatePromptForAgent(context.Background(), prompt, id))
			saved, err := repo.GetPromptByID(context.Background(), id)
			require.NoError(t, err)
			require.Equal(t, "Agent revision", saved.Content)
		})
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
	exists, err := db.ColumnExists(database, "custom_prompts", "allow_agent_edits")
	require.NoError(t, err)
	require.False(t, exists)
	assertAgentWriteGuards(t, repo)
	assertOperatorWriteGuard(t, repo)
	assertSchemaReplay(t, database)
}

func TestPostgresPromptAgentWriteIgnoresHistoricalPermission(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	_, err := database.Exec(`CREATE TABLE custom_prompts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		content TEXT NOT NULL,
		builtin INTEGER NOT NULL DEFAULT 0,
		allow_agent_edits INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`)
	require.NoError(t, err)
	assertLegacyPermissionWrites(t, database)
	repo, err := newSQLiteRepositoryWithDB(database, database)
	require.NoError(t, err)
	for _, id := range []string{"legacy-zero", "legacy-one"} {
		prompt, err := repo.GetPromptByID(context.Background(), id)
		require.NoError(t, err)
		prompt.Content = "Agent revision"
		require.NoError(t, repo.UpdatePromptForAgent(context.Background(), prompt, id))
		saved, err := repo.GetPromptByID(context.Background(), id)
		require.NoError(t, err)
		require.Equal(t, "Agent revision", saved.Content)
	}
}

func assertLegacyPermissionWrites(t *testing.T, database *sqlx.DB) {
	t.Helper()
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, allowed := range []int{0, 1} {
		id := "legacy-zero"
		if allowed == 1 {
			id = "legacy-one"
		}
		_, err := database.Exec(database.Rebind(`INSERT INTO custom_prompts (id,name,content,builtin,allow_agent_edits,created_at,updated_at) VALUES (?,?,?,0,?,?,?)`), id, id, "Keep me", allowed, stamp, stamp)
		require.NoError(t, err)
	}
}

func assertSchemaReplay(t *testing.T, database *sqlx.DB) {
	t.Helper()
	for range 2 {
		_, err := newSQLiteRepositoryWithDB(database, database)
		require.NoError(t, err)
	}
}

func assertAgentWriteGuards(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	for _, mutation := range []string{"rename", "content", "delete", "builtin", "custom"} {
		t.Run(mutation, func(t *testing.T) {
			prompt := &models.Prompt{Name: mutation, Content: "Original", Builtin: mutation == "builtin"}
			require.NoError(t, repo.CreatePrompt(ctx, prompt))
			observed, err := repo.GetPromptByName(ctx, mutation)
			require.NoError(t, err)
			switch mutation {
			case "rename":
				prompt.Name = "renamed"
			case "content":
				prompt.Content = "Human revision"
			}
			if mutation == "delete" {
				require.NoError(t, repo.DeletePrompt(ctx, prompt.ID))
			} else if mutation != "custom" && mutation != "builtin" {
				require.NoError(t, repo.UpdatePrompt(ctx, prompt))
			}
			observed.Content = "Agent revision"
			err = repo.UpdatePromptForAgent(ctx, observed, mutation)
			if mutation == "custom" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrPromptWriteRejected)
			}
			if mutation == "delete" {
				return
			}
			saved, err := repo.GetPromptByID(ctx, prompt.ID)
			require.NoError(t, err)
			if mutation == "custom" {
				require.Equal(t, "Agent revision", saved.Content)
			} else {
				require.Equal(t, prompt.Content, saved.Content)
			}
		})
	}
}

func assertOperatorWriteGuard(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	prompt := &models.Prompt{Name: "operator-race", Content: "Original"}
	require.NoError(t, repo.CreatePrompt(ctx, prompt))
	stale, err := repo.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	prompt.Content = "Operator revision"
	require.NoError(t, repo.UpdatePrompt(ctx, prompt))
	revoked, err := repo.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	stale.Content = "Stale content save"
	require.ErrorIs(t, repo.UpdatePrompt(ctx, stale), ErrPromptWriteRejected)
	saved, err := repo.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	require.Equal(t, "Operator revision", saved.Content)
	require.True(t, revoked.UpdatedAt.Equal(saved.UpdatedAt))
}
