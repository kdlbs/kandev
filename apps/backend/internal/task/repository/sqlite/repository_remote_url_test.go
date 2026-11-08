package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func remoteURLStoreFixture(t *testing.T) (*Repository, *models.Repository) {
	t.Helper()
	r := newRepoForEntityTests(t)
	seedWorkspace(t, r, "remote-url-ws")
	row := &models.Repository{
		ID: "remote-url", WorkspaceID: "remote-url-ws", Name: "acme/widget", SourceType: "provider",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widget",
		RemoteURL: "git@github.com:acme/widget.git", DefaultBranch: "main", PullBeforeWorktree: true,
	}
	require.NoError(t, r.CreateRepository(t.Context(), row))
	return r, row
}

func TestRepositoryRemoteURLStorageOmission(t *testing.T) {
	r, row := remoteURLStoreFixture(t)
	stale, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	repair := "https://github.com/acme/widget.git"
	observed := models.NewRepositoryIdentityFence(stale)
	require.NoError(t, r.UpdateRepositoryWithCheckoutIntent(t.Context(), stale, models.RepositoryCheckoutIntent{
		RemoteURL: &repair, ObservedIdentity: observed,
	}))
	require.Equal(t, repair, stale.RemoteURL)

	ordinary, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	ordinary.Name = "Saved"
	require.NoError(t, r.UpdateRepositoryWithCheckoutIntent(t.Context(), ordinary, models.RepositoryCheckoutIntent{}))
	require.Equal(t, repair, ordinary.RemoteURL, "omitted URL must project the committed value")

	stored, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, repair, stored.RemoteURL)
	require.Equal(t, "Saved", stored.Name)
}

func TestRepositoryRemoteURLStorageIdentityFence(t *testing.T) {
	r, row := remoteURLStoreFixture(t)
	stale, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	observed := models.NewRepositoryIdentityFence(stale)

	other, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	other.ProviderOwner = "other"
	require.NoError(t, r.UpdateRepository(t.Context(), other))

	repair := "https://github.com/acme/widget.git"
	err = r.UpdateRepositoryWithCheckoutIntent(t.Context(), stale, models.RepositoryCheckoutIntent{
		RemoteURL: &repair, ObservedIdentity: observed,
	})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
	stored, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "other", stored.ProviderOwner)
	require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
}

func TestRepositoryRemoteURLAtomicity(t *testing.T) {
	r, row := remoteURLStoreFixture(t)
	_, err := r.db.Exec(`CREATE TRIGGER reject_remote_url_binding BEFORE INSERT ON repository_secret_bindings BEGIN SELECT RAISE(ABORT, 'binding rejected'); END`)
	require.NoError(t, err)

	stale, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	repair := "https://github.com/acme/widget.git"
	err = r.UpdateRepositoryWithSecretBindingsAndCheckoutIntent(t.Context(), stale, []models.RepositorySecretBinding{{Key: "TOKEN", SecretID: "ref"}}, models.RepositoryCheckoutIntent{
		RemoteURL: &repair, ObservedIdentity: models.NewRepositoryIdentityFence(stale),
	})
	require.Error(t, err)

	stored, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
	require.Equal(t, stale.UpdatedAt, stored.UpdatedAt)
	require.Empty(t, stored.SecretBindings)
}

func TestRepositoryRemoteURLLegacyExactWritesKeepURL(t *testing.T) {
	r, row := remoteURLStoreFixture(t)
	legacy, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	legacy.RemoteURL = "https://github.com/acme/widget.git"
	legacy.Name = "legacy"
	require.NoError(t, r.UpdateRepository(t.Context(), legacy))

	exact, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	exact.RemoteURL = "git@github.com:acme/widget.git"
	exact.Name = "exact"
	require.NoError(t, r.UpdateRepositoryIfUnchanged(t.Context(), exact, exact.UpdatedAt))

	stored, err := r.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "git@github.com:acme/widget.git", stored.RemoteURL)
	require.Equal(t, "exact", stored.Name)
}
