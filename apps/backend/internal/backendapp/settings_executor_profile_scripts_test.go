package backendapp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/settingscatalog"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var settingsScriptTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	_, err := tasksqlite.NewWithDB(database, database, nil)
	return err
})

type settingsScriptSnapshot struct {
	*tasksqlite.Repository
	afterRead func(context.Context) error
}

func (r *settingsScriptSnapshot) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	profile, err := r.Repository.GetExecutorProfile(ctx, id)
	if err != nil || r.afterRead == nil {
		return profile, err
	}
	hook := r.afterRead
	r.afterRead = nil
	return profile, hook(ctx)
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.8 AC-EXECUTORS-PROFILE-EDITOR-001.9 AC-EXECUTORS-PROFILE-EDITOR-001.11
func TestExecutorProfileScriptsSettingsDomain(t *testing.T) {
	a, path := settingsScriptTemplate.Open(t)
	raw, err := db.OpenSQLite(path)
	require.NoError(t, err)
	b := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	repo := tasksqlite.NewWithInitializedDB(a, a, nil)
	otherRepo := tasksqlite.NewWithInitializedDB(b, b, nil)
	require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{ID: "settings-executor", Name: "Local", Type: models.ExecutorTypeLocal}))
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), &models.ExecutorProfile{ID: "settings-profile", ExecutorID: "settings-executor", Name: "Before", PrepareScript: "old-prepare", CleanupScript: "old-cleanup"}))
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	gate := &settingsScriptSnapshot{Repository: repo}
	svc := service.NewService(service.Repos{Executors: gate}, nil, log, service.RepositoryDiscoveryConfig{})
	other := service.NewService(service.Repos{Executors: otherRepo}, nil, log, service.RepositoryDiscoveryConfig{})
	catalog, err := settingscatalog.DefaultRegistry()
	require.NoError(t, err)
	h := mcphandlers.NewHandlers(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)
	h.SetSettingsCatalog(catalog)
	h.SetSettingsOperations(newSettingsOperations(catalog, nil, nil, settingsDomainDependencies{task: svc}))
	d := ws.NewDispatcher()
	h.RegisterHandlers(d)
	prepare, cleanup := "new-prepare", "new-cleanup"
	gate.afterRead = func(ctx context.Context) error {
		_, err := other.UpdateExecutorProfile(ctx, "settings-profile", &service.UpdateExecutorProfileRequest{PrepareScript: &prepare, CleanupScript: &cleanup})
		return err
	}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{Synthetic: true})
	response := callProfileScriptSettings(t, d, ctx, "settings-profile", map[string]any{"name": "Edited"})
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	var result struct {
		Settings models.ExecutorProfile `json:"settings"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &result))
	stored, err := repo.GetExecutorProfile(ctx, "settings-profile")
	require.NoError(t, err)
	require.Equal(t, prepare, result.Settings.PrepareScript)
	require.Equal(t, cleanup, result.Settings.CleanupScript)
	require.Equal(t, prepare, stored.PrepareScript)
	require.Equal(t, cleanup, stored.CleanupScript)
	response = callProfileScriptSettings(t, d, ctx, "settings-profile", map[string]any{"prepare_script": "", "cleanup_script": ""})
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	stored, err = repo.GetExecutorProfile(ctx, "settings-profile")
	require.NoError(t, err)
	require.Empty(t, stored.PrepareScript)
	require.Empty(t, stored.CleanupScript)
	response = callProfileScriptSettings(t, d, ctx, "missing-profile", map[string]any{"name": "Rejected"})
	require.Equal(t, ws.MessageTypeError, response.Type)
}

func callProfileScriptSettings(t *testing.T, d *ws.Dispatcher, ctx context.Context, id string, changes map[string]any) *ws.Message {
	t.Helper()
	req, err := ws.NewRequest("profile-scripts", ws.ActionMCPUpdateSettings, map[string]any{
		"target": map[string]any{"resource_type": "executor_profile", "resource_id": id}, "changes": changes,
	})
	require.NoError(t, err)
	response, err := d.Dispatch(ctx, req)
	require.NoError(t, err)
	return response
}
