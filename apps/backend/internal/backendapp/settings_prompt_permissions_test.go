package backendapp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/stretchr/testify/require"
)

func TestSettingsPromptWritesAllowCustomPrompts(t *testing.T) {
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "prompts.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, _, err := promptstore.Provide(database, database)
	require.NoError(t, err)
	svc := promptservice.NewService(repo)
	operations := &settingsOperations{deps: settingsDomainDependencies{prompts: svc}}
	ctx := context.Background()
	prompt, err := svc.CreatePrompt(ctx, "custom", "Original")
	require.NoError(t, err)
	patch := map[string]json.RawMessage{"content": json.RawMessage(`"Changed"`)}
	updated, err := operations.updatePrompt(ctx, prompt.ID, patch)
	require.NoError(t, err)
	require.Equal(t, "Changed", updated.(*models.Prompt).Content)
	persisted, err := svc.GetPromptByName(ctx, prompt.Name)
	require.NoError(t, err)
	require.Equal(t, "Changed", persisted.Content)
	read, err := operations.readPrompt(ctx, prompt.ID)
	require.NoError(t, err)
	require.Equal(t, true, read["allow_agent_edits"])
	_, err = operations.updatePrompt(ctx, prompt.ID, map[string]json.RawMessage{
		"allow_agent_edits": json.RawMessage(`false`),
	})
	require.ErrorContains(t, err, "allow_agent_edits")
	persisted, err = svc.GetPromptByName(ctx, prompt.Name)
	require.NoError(t, err)
	require.Equal(t, "Changed", persisted.Content)
	builtin, err := svc.GetPromptByName(ctx, "code-review")
	require.NoError(t, err)
	_, err = operations.updatePrompt(ctx, builtin.ID, patch)
	require.ErrorIs(t, err, promptservice.ErrBuiltinPrompt)
	read, err = operations.readPrompt(ctx, builtin.ID)
	require.NoError(t, err)
	require.Equal(t, false, read["allow_agent_edits"])
}
