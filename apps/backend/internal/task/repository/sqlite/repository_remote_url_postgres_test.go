package sqlite_test

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

func remoteURLPostgresSeed(t *testing.T) (*tasksqlite.Repository, *tasksqlite.Repository) {
	t.Helper()
	a, b, _ := workflowPostgresPair(t)
	require.NoError(t, a.CreateWorkspace(t.Context(), &models.Workspace{ID: "remote-url-ws", Name: "Workspace"}))
	require.NoError(t, a.CreateRepository(t.Context(), &models.Repository{
		ID: "remote-url", WorkspaceID: "remote-url-ws", Name: "acme/widget", SourceType: "provider",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widget",
		RemoteURL: "git@github.com:acme/widget.git", DefaultBranch: "main", PullBeforeWorktree: true,
	}))
	require.NotEqual(t, checkoutPostgresPID(t, a.DB()), checkoutPostgresPID(t, b.DB()))
	return a, b
}

func TestRepositoryRemoteURLPhysicalPostgres(t *testing.T) {
	for _, held := range []int{0, 1} {
		t.Run(fmt.Sprintf("ordinary_save_and_repair/held_%d", held), func(t *testing.T) {
			a, b := remoteURLPostgresSeed(t)
			stores := [2]*tasksqlite.Repository{a, b}
			services := [2]*service.Service{}
			publishers := [2]*pgCheckoutPublisher{}
			for index := range stores {
				services[index], publishers[index] = pgCheckoutAPI(t, stores[index])
			}
			gate := &pgCheckoutReadGate{Repository: stores[held]}
			services[held] = service.NewService(service.Repos{Workspaces: stores[held], RepoEntities: gate}, publishers[held], newCheckoutPGLogger(t), service.RepositoryDiscoveryConfig{})

			repair := "https://github.com/acme/widget.git"
			requests := [2]*service.UpdateRepositoryRequest{{Name: pgCheckoutPtr("Saved")}, {RemoteURL: &repair}}
			gate.after = func() {
				_, err := services[1-held].UpdateRepository(t.Context(), "remote-url", requests[1-held])
				require.NoError(t, err)
			}
			row, err := services[held].UpdateRepository(t.Context(), "remote-url", requests[held])
			require.NoError(t, err)
			require.Equal(t, repair, row.RemoteURL)

			stored, err := stores[0].GetRepository(t.Context(), "remote-url")
			require.NoError(t, err)
			require.Equal(t, repair, stored.RemoteURL)
			require.NotEmpty(t, publishers[held].rows)
			values := publishers[held].rows[len(publishers[held].rows)-1].Data.(map[string]interface{})
			require.Equal(t, repair, values["remote_url"])
		})
	}

	t.Run("identity_change_during_inspection_conflicts", func(t *testing.T) {
		a, b := remoteURLPostgresSeed(t)
		_, firstEvents := pgCheckoutAPI(t, a)
		second, _ := pgCheckoutAPI(t, b)
		gate := &pgCheckoutReadGate{Repository: a}
		first := service.NewService(service.Repos{Workspaces: a, RepoEntities: gate}, firstEvents, newCheckoutPGLogger(t), service.RepositoryDiscoveryConfig{})
		gate.after = func() {
			owner := "other"
			_, err := second.UpdateRepository(t.Context(), "remote-url", &service.UpdateRepositoryRequest{ProviderOwner: &owner})
			require.NoError(t, err)
		}
		repair := "https://github.com/acme/widget.git"
		_, err := first.UpdateRepository(t.Context(), "remote-url", &service.UpdateRepositoryRequest{RemoteURL: &repair})
		require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
		stored, err := a.GetRepository(t.Context(), "remote-url")
		require.NoError(t, err)
		require.Equal(t, "other", stored.ProviderOwner)
		require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
	})
}
