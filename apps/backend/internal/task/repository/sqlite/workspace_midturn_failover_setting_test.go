package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

func TestWorkspaceMidTurnFailoverSettingDefaultsOffAndRoundTrips(t *testing.T) {
	database, _ := workspaceSettingsStorageTemplate.Open(t)
	repo := tasksqlite.NewWithInitializedDB(database, database, nil)
	api := workspaceSettingsAPI(t, repo)
	ctx := t.Context()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "midturn-setting", Name: "Failover"}))

	created, err := repo.GetWorkspace(ctx, "midturn-setting")
	require.NoError(t, err)
	require.False(t, created.DynamicMidTurnFailoverEnabled, "the setting is off by default")

	enabled := true
	updated, err := api.UpdateWorkspace(ctx, created.ID, &service.UpdateWorkspaceRequest{DynamicMidTurnFailoverEnabled: &enabled})
	require.NoError(t, err)
	require.True(t, updated.DynamicMidTurnFailoverEnabled)

	name := "Renamed"
	renamed, err := api.UpdateWorkspace(ctx, created.ID, &service.UpdateWorkspaceRequest{Name: &name})
	require.NoError(t, err)
	require.True(t, renamed.DynamicMidTurnFailoverEnabled, "an omitted field keeps the saved value")

	listed, err := repo.ListWorkspaces(ctx)
	require.NoError(t, err)
	listedWorkspace := findListedWorkspace(t, listed, created.ID)
	require.True(t, listedWorkspace.DynamicMidTurnFailoverEnabled)

	full := *renamed
	full.DynamicMidTurnFailoverEnabled = false
	require.NoError(t, repo.UpdateWorkspace(ctx, &full))
	stored, err := repo.GetWorkspace(ctx, created.ID)
	require.NoError(t, err)
	require.False(t, stored.DynamicMidTurnFailoverEnabled, "a full-row update writes the field")
}
