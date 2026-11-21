package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

func TestWorkspaceFailedRouteAutoRetrySettingDefaultsAndRoundTrips(t *testing.T) {
	database, _ := workspaceSettingsStorageTemplate.Open(t)
	repo := tasksqlite.NewWithInitializedDB(database, database, nil)
	api := workspaceSettingsAPI(t, repo)
	ctx := t.Context()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "auto-retry-setting", Name: "Retry"}))

	created, err := repo.GetWorkspace(ctx, "auto-retry-setting")
	require.NoError(t, err)
	require.False(t, created.DynamicFailedRouteAutoRetryEnabled, "the setting is off by default")
	require.Equal(t, 1, created.DynamicFailedRouteAutoRetryMax, "the limit defaults to one retry")
	require.Zero(t, created.FailedRouteAutoRetryLimit(), "an off setting allows no retries")

	enabled, limit := true, 3
	updated, err := api.UpdateWorkspace(ctx, created.ID, &service.UpdateWorkspaceRequest{
		DynamicFailedRouteAutoRetryEnabled: &enabled, DynamicFailedRouteAutoRetryMax: &limit,
	})
	require.NoError(t, err)
	require.True(t, updated.DynamicFailedRouteAutoRetryEnabled)
	require.Equal(t, 3, updated.FailedRouteAutoRetryLimit())

	name := "Renamed"
	renamed, err := api.UpdateWorkspace(ctx, created.ID, &service.UpdateWorkspaceRequest{Name: &name})
	require.NoError(t, err)
	require.True(t, renamed.DynamicFailedRouteAutoRetryEnabled, "an omitted field keeps the saved value")
	require.Equal(t, 3, renamed.DynamicFailedRouteAutoRetryMax)

	for _, invalid := range []int{0, 4, -1} {
		value := invalid
		_, err := api.UpdateWorkspace(ctx, created.ID, &service.UpdateWorkspaceRequest{DynamicFailedRouteAutoRetryMax: &value})
		require.ErrorIs(t, err, service.ErrWorkspaceFailedRouteAutoRetryMaxInvalid, "limit %d", invalid)
	}

	listed, err := repo.ListWorkspaces(ctx)
	require.NoError(t, err)
	listedWorkspace := findListedWorkspace(t, listed, created.ID)
	require.True(t, listedWorkspace.DynamicFailedRouteAutoRetryEnabled)
	require.Equal(t, 3, listedWorkspace.DynamicFailedRouteAutoRetryMax)

	full := *renamed
	full.DynamicFailedRouteAutoRetryEnabled = false
	full.DynamicFailedRouteAutoRetryMax = 2
	require.NoError(t, repo.UpdateWorkspace(ctx, &full))
	stored, err := repo.GetWorkspace(ctx, created.ID)
	require.NoError(t, err)
	require.False(t, stored.DynamicFailedRouteAutoRetryEnabled, "a full-row update writes the toggle")
	require.Equal(t, 2, stored.DynamicFailedRouteAutoRetryMax, "a full-row update writes the limit")
}

// findListedWorkspace returns one workspace from a listing; the storage
// template also holds the default workspace.
func findListedWorkspace(t *testing.T, listed []*models.Workspace, id string) *models.Workspace {
	t.Helper()
	for _, workspace := range listed {
		if workspace.ID == id {
			return workspace
		}
	}
	t.Fatalf("workspace %s is not listed", id)
	return nil
}
