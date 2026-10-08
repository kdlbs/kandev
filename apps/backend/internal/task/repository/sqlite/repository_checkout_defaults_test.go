package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func checkoutStorePtr[T any](v T) *T { return &v }
func checkoutStoreFixture(t *testing.T) (*Repository, *models.Repository) {
	t.Helper()
	r := newRepoForEntityTests(t)
	seedWorkspace(t, r, "checkout-ws")
	row := &models.Repository{ID: "checkout", WorkspaceID: "checkout-ws", Name: "Before", DefaultBranch: "main", PullBeforeWorktree: true, SetupScript: "keep setup", CopyFiles: "*.json"}
	require.NoError(t, r.CreateRepository(t.Context(), row))
	return r, row
}

func checkoutStoredVersion(t *testing.T, r *Repository, id string, version time.Time) *models.Repository {
	t.Helper()
	_, err := r.db.ExecContext(t.Context(), `UPDATE repositories SET updated_at = ? WHERE id = ?`, version, id)
	require.NoError(t, err)
	row, err := r.GetRepository(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, version, row.UpdatedAt)
	return row
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.21
func TestRepositoryCheckoutDefaultsStorage(t *testing.T) {
	r, stale := checkoutStoreFixture(t)
	other := *stale
	other.DefaultBranch = "develop"
	other.PullBeforeWorktree = false
	require.NoError(t, r.UpdateRepository(t.Context(), &other))
	stale.Name = "Saved"
	require.NoError(t, r.UpdateRepositoryWithCheckoutIntent(t.Context(), stale, models.RepositoryCheckoutIntent{}))
	require.Equal(t, "develop", stale.DefaultBranch)
	require.False(t, stale.PullBeforeWorktree)
	stored, err := r.GetRepository(t.Context(), stale.ID)
	require.NoError(t, err)
	require.Equal(t, stale.DefaultBranch, stored.DefaultBranch)
	require.Equal(t, stale.PullBeforeWorktree, stored.PullBeforeWorktree)
	require.Equal(t, stale.UpdatedAt, stored.UpdatedAt)
	require.Equal(t, "Saved", stored.Name)
	require.Equal(t, "*.json", stored.CopyFiles)
	require.Equal(t, "keep setup", stored.SetupScript)
	require.NoError(t, r.UpdateRepositoryWithCheckoutIntent(t.Context(), stale, models.RepositoryCheckoutIntent{DefaultBranch: checkoutStorePtr("")}))
	require.Empty(t, stale.DefaultBranch)
	require.False(t, stale.PullBeforeWorktree)
	require.NoError(t, r.UpdateRepositoryWithCheckoutIntent(t.Context(), stale, models.RepositoryCheckoutIntent{PullBeforeWorktree: checkoutStorePtr(true)}))
	require.Empty(t, stale.DefaultBranch)
	require.True(t, stale.PullBeforeWorktree)
	before, err := r.GetRepository(t.Context(), stale.ID)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, r.UpdateRepositoryWithCheckoutIntent(ctx, stale, models.RepositoryCheckoutIntent{DefaultBranch: checkoutStorePtr("rejected")}))
	after, err := r.GetRepository(t.Context(), stale.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, r.DeleteRepository(t.Context(), stale.ID))
	require.ErrorContains(t, r.UpdateRepositoryWithCheckoutIntent(t.Context(), stale, models.RepositoryCheckoutIntent{}), "repository not found")
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
func TestRepositoryCheckoutDefaultsLegacyAndExact(t *testing.T) {
	for _, bindings := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary_exact", true: "binding_exact"}[bindings], func(t *testing.T) {
			r, original := checkoutStoreFixture(t)
			original = checkoutStoredVersion(t, r, original.ID, time.Unix(1000, 0).UTC())
			initialVersion := original.UpdatedAt
			legacy := *original
			legacy.DefaultBranch = "release"
			legacy.PullBeforeWorktree = false
			require.NoError(t, r.UpdateRepository(t.Context(), &legacy))
			stored, err := r.GetRepository(t.Context(), original.ID)
			require.NoError(t, err)
			require.Equal(t, "release", stored.DefaultBranch)
			require.False(t, stored.PullBeforeWorktree)
			exact := *stored
			exact.DefaultBranch = "exact"
			exact.PullBeforeWorktree = true
			if bindings {
				err = r.UpdateRepositoryWithSecretBindingsIfUnchanged(t.Context(), &exact, []models.RepositorySecretBinding{}, stored.UpdatedAt)
			} else {
				err = r.UpdateRepositoryIfUnchanged(t.Context(), &exact, stored.UpdatedAt)
			}
			require.NoError(t, err)
			rejected := exact
			rejected.Name = "Rejected"
			if bindings {
				err = r.UpdateRepositoryWithSecretBindingsIfUnchanged(t.Context(), &rejected, nil, initialVersion)
			} else {
				err = r.UpdateRepositoryIfUnchanged(t.Context(), &rejected, initialVersion)
			}
			require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
			exact = *checkoutStoredVersion(t, r, original.ID, time.Unix(2000, 0).UTC())
			version := exact.UpdatedAt
			exact.Name = "Disjoint"
			require.NoError(t, r.UpdateRepositoryWithCheckoutIntent(t.Context(), &exact, models.RepositoryCheckoutIntent{}))
			require.NotEqual(t, version, exact.UpdatedAt)
			if bindings {
				err = r.UpdateRepositoryWithSecretBindingsIfUnchanged(t.Context(), &rejected, nil, version)
			} else {
				err = r.UpdateRepositoryIfUnchanged(t.Context(), &rejected, version)
			}
			require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
			require.NoError(t, r.UpdateRepositoryDefaultBranch(t.Context(), original.ID, "exact", "recovered"))
			require.Error(t, r.UpdateRepositoryDefaultBranch(t.Context(), original.ID, "exact", "wrong"))
			stored, err = r.GetRepository(t.Context(), original.ID)
			require.NoError(t, err)
			require.Equal(t, "recovered", stored.DefaultBranch)
			require.True(t, stored.PullBeforeWorktree)
			require.Equal(t, "Disjoint", stored.Name)
			require.NotEqual(t, "Rejected", stored.Name)
		})
	}
	t.Run("legacy_companion_failure", func(t *testing.T) {
		r, row := checkoutStoreFixture(t)
		before := *row
		_, err := r.db.Exec(`CREATE TRIGGER reject_legacy_checkout BEFORE INSERT ON repository_secret_bindings BEGIN SELECT RAISE(ABORT, 'legacy binding rejected'); END`)
		require.NoError(t, err)
		row.DefaultBranch = "rejected"
		row.PullBeforeWorktree = false
		require.Error(t, r.UpdateRepositoryWithSecretBindings(t.Context(), row, []models.RepositorySecretBinding{{Key: "TOKEN", SecretID: "ref"}}))
		stored, err := r.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		require.Equal(t, before.DefaultBranch, stored.DefaultBranch)
		require.Equal(t, before.PullBeforeWorktree, stored.PullBeforeWorktree)
		require.Equal(t, before.UpdatedAt, stored.UpdatedAt)
	})
}
