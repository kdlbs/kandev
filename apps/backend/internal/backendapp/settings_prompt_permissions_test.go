package backendapp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/stretchr/testify/require"
)

func TestSettingsPromptWritesRespectAgentPermission(t *testing.T) {
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "prompts.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, _, err := promptstore.Provide(database, database)
	require.NoError(t, err)
	svc := promptservice.NewService(repo)
	operations := &settingsOperations{deps: settingsDomainDependencies{prompts: svc}}
	ctx := context.Background()
	prompt, err := svc.CreatePrompt(ctx, "protected", "Original")
	require.NoError(t, err)
	patch := map[string]json.RawMessage{"content": json.RawMessage(`"Changed"`)}
	_, err = operations.updatePrompt(ctx, prompt.ID, patch)
	require.ErrorIs(t, err, promptservice.ErrPromptAgentEditsDisabled)
	allowed := true
	_, err = svc.UpdatePromptWithPermission(ctx, prompt.ID, nil, nil, &allowed)
	require.NoError(t, err)
	_, err = operations.updatePrompt(ctx, prompt.ID, patch)
	require.NoError(t, err)
	persisted, err := svc.GetPromptByName(ctx, prompt.Name)
	require.NoError(t, err)
	require.Equal(t, "Changed", persisted.Content)
	builtin, err := svc.GetPromptByName(ctx, "code-review")
	require.NoError(t, err)
	_, err = operations.updatePrompt(ctx, builtin.ID, patch)
	require.ErrorIs(t, err, promptservice.ErrBuiltinPrompt)
}
