package sqlite_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
)

var workspaceSettingsStorageTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	_, err := tasksqlite.NewWithDB(database, database, nil)
	return err
})

func workspaceSettingsAPI(t *testing.T, repo *tasksqlite.Repository) *service.Service {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	return service.NewService(service.Repos{Workspaces: repo}, nil, log, service.RepositoryDiscoveryConfig{})
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.2, AC-WORKSPACES-SETTINGS-UPDATES-001.3, AC-WORKSPACES-SETTINGS-UPDATES-001.5, AC-WORKSPACES-SETTINGS-UPDATES-001.6, AC-WORKSPACES-SETTINGS-UPDATES-001.8
func TestWorkspaceFieldUpdatesStorage(t *testing.T) {
	database, _ := workspaceSettingsStorageTemplate.Open(t)
	repo := tasksqlite.NewWithInitializedDB(database, database, nil)
	api := workspaceSettingsAPI(t, repo)
	ctx := t.Context()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "settings-storage", Name: "Before", Description: "Kept"}))
	before, err := repo.GetWorkspace(ctx, "settings-storage")
	require.NoError(t, err)
	require.False(t, before.ACPIdleSuspensionEnabled)
	require.Equal(t, 120, before.ACPIdleTimeoutMinutes)
	name := "Partial name"
	row, err := api.UpdateWorkspace(ctx, before.ID, &service.UpdateWorkspaceRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "Kept", row.Description)
	var nullDefaults bool
	require.NoError(t, repo.DB().QueryRowContext(ctx, `SELECT default_executor_id IS NULL AND default_environment_id IS NULL AND default_agent_profile_id IS NULL AND default_config_agent_profile_id IS NULL FROM workspaces WHERE id='settings-storage'`).Scan(&nullDefaults))
	require.True(t, nullDefaults, "untouched nullable columns keep their storage representation")
	stored, err := repo.GetWorkspace(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, row, stored)
	workspaceSettingsDirectStorage(t, repo, stored)
	stored, err = repo.GetWorkspace(ctx, row.ID)
	require.NoError(t, err)
	exact := *stored
	exact.Name, exact.Description = "Complete name", "Complete description"
	exact.ACPIdleTimeoutMinutes = 0
	require.NoError(t, repo.UpdateWorkspaceIfUnchanged(ctx, &exact, stored.UpdatedAt))
	complete, err := repo.GetWorkspace(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, "Complete name", complete.Name)
	require.Equal(t, "Complete description", complete.Description)
	require.Equal(t, 120, complete.ACPIdleTimeoutMinutes, "deliberate complete writer retains its zero-timeout default")
	require.ErrorIs(t, repo.UpdateWorkspaceIfUnchanged(ctx, before, before.UpdatedAt), repoerrors.ErrTaskVersionConflict)
	_, err = api.UpdateWorkspace(ctx, "missing-settings", &service.UpdateWorkspaceRequest{Name: &name})
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
	require.ErrorIs(t, repo.UpdateWorkspaceIfUnchanged(ctx, &models.Workspace{ID: "missing-settings"}, complete.UpdatedAt), repoerrors.ErrTaskVersionConflict)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = api.UpdateWorkspace(cancelled, complete.ID, &service.UpdateWorkspaceRequest{Name: &name})
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_storage_settings BEFORE UPDATE ON workspaces BEGIN SELECT RAISE(ABORT, 'storage settings rejected'); END`)
	require.NoError(t, err)
	enabled := true
	_, err = api.UpdateWorkspace(ctx, complete.ID, &service.UpdateWorkspaceRequest{Name: &name, ACPIdleSuspensionEnabled: &enabled})
	require.ErrorContains(t, err, "storage settings rejected")
	stored, err = repo.GetWorkspace(ctx, complete.ID)
	require.NoError(t, err)
	require.Equal(t, complete, stored, "statement abort preserves values and exact timestamp")
}

func workspaceSettingsDirectStorage(t *testing.T, repo *tasksqlite.Repository, before *models.Workspace) {
	t.Helper()
	name, description, unit := "", "Direct description", "storage-unit"
	executor, environment, agent, config := "executor", "environment", "agent", "config"
	executorID, environmentID, agentID, configID := &executor, &environment, &agent, &config
	enabled, timeout := true, 31
	update := models.WorkspaceFieldUpdate{
		Name: &name, Description: &description, UnitID: &unit,
		DefaultExecutorID: &executorID, DefaultEnvironmentID: &environmentID,
		DefaultAgentProfileID: &agentID, DefaultConfigAgentProfileID: &configID,
		ACPIdleSuspensionEnabled: &enabled, ACPIdleTimeoutMinutes: &timeout,
	}
	row, err := repo.UpdateWorkspaceFields(t.Context(), before.ID, update, &before.UpdatedAt)
	require.NoError(t, err)
	require.Empty(t, row.Name)
	require.Equal(t, description, row.Description)
	require.Equal(t, unit, row.UnitID)
	require.Equal(t, executorID, row.DefaultExecutorID)
	require.Equal(t, environmentID, row.DefaultEnvironmentID)
	require.Equal(t, agentID, row.DefaultAgentProfileID)
	require.Equal(t, configID, row.DefaultConfigAgentProfileID)
	require.True(t, row.ACPIdleSuspensionEnabled)
	require.Equal(t, timeout, row.ACPIdleTimeoutMinutes)
	require.True(t, row.UpdatedAt.After(before.UpdatedAt))
	stored, err := repo.GetWorkspace(t.Context(), before.ID)
	require.NoError(t, err)
	require.Equal(t, row, stored)
	_, err = repo.UpdateWorkspaceFields(t.Context(), before.ID, update, &before.UpdatedAt)
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
	var clear *string
	disabled := false
	row, err = repo.UpdateWorkspaceFields(t.Context(), before.ID, models.WorkspaceFieldUpdate{
		DefaultExecutorID: &clear, DefaultEnvironmentID: &clear,
		DefaultAgentProfileID: &clear, DefaultConfigAgentProfileID: &clear,
		ACPIdleSuspensionEnabled: &disabled,
	}, nil)
	require.NoError(t, err)
	require.False(t, row.ACPIdleSuspensionEnabled)
	require.Equal(t, timeout, row.ACPIdleTimeoutMinutes)
	var allNull bool
	require.NoError(t, repo.DB().QueryRowContext(t.Context(), `SELECT default_executor_id IS NULL AND default_environment_id IS NULL AND default_agent_profile_id IS NULL AND default_config_agent_profile_id IS NULL FROM workspaces WHERE id='settings-storage'`).Scan(&allNull))
	require.True(t, allNull)
	_, err = repo.UpdateWorkspaceFields(t.Context(), "missing-settings", update, nil)
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
	_, err = repo.UpdateWorkspaceFields(t.Context(), "missing-settings", update, &row.UpdatedAt)
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
}
