package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.21
func TestRepositoryCheckoutDefaultsPresence(t *testing.T) {
	f := checkoutServices(t, false)
	created, err := f[0].api.CreateRepository(t.Context(), &CreateRepositoryRequest{WorkspaceID: "checkout-ws", Name: "Created"})
	require.NoError(t, err)
	require.True(t, created.PullBeforeWorktree)
	f[0].bus.ClearEvents()
	for _, payload := range []string{`{}`, `{"default_branch":null,"pull_before_worktree":null}`, `{"default_branch":"","pull_before_worktree":false}`, `{"default_branch":"topic","pull_before_worktree":true}`, `{"pull_before_worktree":false}`} {
		// A historical baseline keeps timestamp advancement independent of clock resolution.
		baseline := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
		result, err := f[0].db.ExecContext(t.Context(), "UPDATE repositories SET updated_at = ? WHERE id = ?", baseline, "checkout-repo")
		require.NoError(t, err)
		affected, err := result.RowsAffected()
		require.NoError(t, err)
		require.EqualValues(t, 1, affected)
		before, err := f[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
		require.NoError(t, err)
		require.True(t, before.UpdatedAt.Equal(baseline))
		var request UpdateRepositoryRequest
		require.NoError(t, json.Unmarshal([]byte(payload), &request))
		row, err := f[0].api.UpdateRepository(t.Context(), before.ID, &request)
		require.NoError(t, err)
		expectedBranch, expectedPull := before.DefaultBranch, before.PullBeforeWorktree
		if request.DefaultBranch != nil {
			expectedBranch = *request.DefaultBranch
		}
		if request.PullBeforeWorktree != nil {
			expectedPull = *request.PullBeforeWorktree
		}
		require.Equal(t, expectedBranch, row.DefaultBranch)
		require.Equal(t, expectedPull, row.PullBeforeWorktree)
		require.True(t, row.UpdatedAt.After(before.UpdatedAt))
		checkoutAssertEvent(t, f[0], row)
		f[0].bus.ClearEvents()
	}
}
func checkoutSecrets(t *testing.T, f [2]checkoutServiceFixture) string {
	t.Helper()
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	store, closeStore, err := secrets.Provide(f[0].db, f[0].db, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeStore()) })
	item := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "checkout-secret", Scope: secrets.ScopeWorkspace, WorkspaceID: "checkout-ws"}, Value: "disposable-value"}
	require.NoError(t, store.Create(t.Context(), item))
	for i := range f {
		f[i].api.SetSecretStore(store)
	}
	return item.ID
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
func TestRepositoryCheckoutDefaultsCompanionAtomicity(t *testing.T) {
	t.Run("stale_save_and_binding_presence", func(t *testing.T) {
		f := checkoutServices(t, true)
		sid := checkoutSecrets(t, f)
		bindings := []RepositorySecretBindingInput{{Key: "TOKEN", SecretID: sid}}
		results := checkoutOverlap(t, f, 0, [2]*UpdateRepositoryRequest{{Name: checkoutPtr("With bindings"), SecretBindings: &bindings}, {DefaultBranch: checkoutPtr("develop"), PullBeforeWorktree: checkoutPtr(false)}})
		require.NoError(t, results[0].err)
		require.Equal(t, "develop", results[0].row.DefaultBranch)
		require.False(t, results[0].row.PullBeforeWorktree)
		require.Len(t, results[0].row.SecretBindings, 1)
		f[0].bus.ClearEvents()
		row, err := f[0].api.UpdateRepository(t.Context(), "checkout-repo", &UpdateRepositoryRequest{})
		require.NoError(t, err)
		require.Len(t, row.SecretBindings, 1)
		empty := []RepositorySecretBindingInput{}
		row, err = f[0].api.UpdateRepository(t.Context(), "checkout-repo", &UpdateRepositoryRequest{SecretBindings: &empty})
		require.NoError(t, err)
		require.Empty(t, row.SecretBindings)
		stored, err := f[1].gate.Repository.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		require.Empty(t, stored.SecretBindings)
		require.Equal(t, "develop", stored.DefaultBranch)
		require.False(t, stored.PullBeforeWorktree)
	})
	t.Run("insert_failure_rolls_back_settings_and_bindings", func(t *testing.T) {
		f := checkoutServices(t, true)
		sid := checkoutSecrets(t, f)
		row, err := f[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
		require.NoError(t, err)
		require.NoError(t, f[0].gate.UpdateRepositoryWithSecretBindings(t.Context(), row, []models.RepositorySecretBinding{{Key: "KEEP", SecretID: sid}}))
		before, err := f[1].gate.Repository.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		_, err = f[0].db.Exec(`CREATE TRIGGER reject_checkout_binding BEFORE INSERT ON repository_secret_bindings WHEN NEW.key = 'FAIL' BEGIN SELECT RAISE(ABORT, 'checkout binding rejected'); END`)
		require.NoError(t, err)
		bindings := []RepositorySecretBindingInput{{Key: "FAIL", SecretID: sid}}
		result, err := f[0].api.UpdateRepository(t.Context(), row.ID, &UpdateRepositoryRequest{Name: checkoutPtr("Rejected"), DefaultBranch: checkoutPtr("develop"), PullBeforeWorktree: checkoutPtr(false), SecretBindings: &bindings})
		require.ErrorContains(t, err, "checkout binding rejected")
		require.Nil(t, result)
		after, err := f[1].gate.Repository.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		require.Equal(t, before, after)
		require.Empty(t, f[0].bus.GetPublishedEvents())
	})
}

type checkoutCommittedGate struct {
	*sqliterepo.Repository
	observed, release chan struct{}
}

func (g *checkoutCommittedGate) UpdateRepositoryWithCheckoutIntent(ctx context.Context, row *models.Repository, intent models.RepositoryCheckoutIntent) error {
	if err := g.Repository.UpdateRepositoryWithCheckoutIntent(ctx, row, intent); err != nil {
		return err
	}
	close(g.observed)
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
func TestRepositoryCheckoutDefaultsOwnMutationProjection(t *testing.T) {
	f := checkoutServices(t, false)
	gate := &checkoutCommittedGate{Repository: f[0].gate.Repository, observed: make(chan struct{}), release: make(chan struct{})}
	f[0].api.repoEntities = gate
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	var workers sync.WaitGroup
	var released sync.Once
	defer func() { released.Do(func() { close(gate.release) }); cancel(); workers.Wait() }()
	result := make(chan checkoutOutcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		row, err := f[0].api.UpdateRepository(ctx, "checkout-repo", &UpdateRepositoryRequest{Name: checkoutPtr("Own mutation")})
		result <- checkoutOutcome{row, err}
	}()
	select {
	case <-gate.observed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	later, err := f[1].api.UpdateRepository(ctx, "checkout-repo", &UpdateRepositoryRequest{DefaultBranch: checkoutPtr("later"), PullBeforeWorktree: checkoutPtr(true)})
	require.NoError(t, err)
	released.Do(func() { close(gate.release) })
	select {
	case first := <-result:
		require.NoError(t, first.err)
		require.Equal(t, "main", first.row.DefaultBranch)
		require.False(t, first.row.PullBeforeWorktree)
		checkoutAssertEvent(t, f[0], first.row)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	checkoutAssertEvent(t, f[1], later)
	stored, err := f[1].gate.Repository.GetRepository(ctx, "checkout-repo")
	require.NoError(t, err)
	require.Equal(t, "later", stored.DefaultBranch)
	require.True(t, stored.PullBeforeWorktree)
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
func TestRepositoryCheckoutDefaultsFailures(t *testing.T) {
	for _, failure := range []string{"invalid_branch", "invalid_provider_pair", "cancelled", "unauthorized", "deleted", "missing", "bindings_unavailable"} {
		t.Run(failure, func(t *testing.T) {
			f := checkoutServices(t, true)
			before, err := f[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
			require.NoError(t, err)
			ctx := t.Context()
			req := &UpdateRepositoryRequest{Name: checkoutPtr("Rejected")}
			id := "checkout-repo"
			switch failure {
			case "invalid_branch":
				req.DefaultBranch = checkoutPtr("bad..branch")
			case "invalid_provider_pair":
				req.ProviderScope = checkoutPtr("scope")
				req.ProviderRepoID = checkoutPtr("")
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "unauthorized":
				claimErr := f[0].gate.ClaimUnownedWorkspaces(t.Context(), "owner")
				require.NoError(t, claimErr)
				ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "foreign"})
			case "deleted":
				require.NoError(t, f[0].gate.DeleteRepository(t.Context(), id))
			case "missing":
				id = "absent"
			case "bindings_unavailable":
				sid := checkoutSecrets(t, f)
				bindings := []RepositorySecretBindingInput{{Key: "TOKEN", SecretID: sid}}
				req.SecretBindings = &bindings
				f[0].api.repoEntities = struct {
					taskrepo.RepositoryEntityRepository
				}{f[0].gate.Repository}
			}
			result, err := f[0].api.UpdateRepository(ctx, id, req)
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, f[0].bus.GetPublishedEvents())
			if failure == "unauthorized" {
				require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
			}
			if failure == "bindings_unavailable" {
				require.ErrorIs(t, err, ErrInvalidRepositorySettings)
			}
			if failure != "deleted" && failure != "missing" {
				after, readErr := f[1].gate.Repository.GetRepository(t.Context(), id)
				require.NoError(t, readErr)
				require.Equal(t, before, after)
			}
		})
	}
}
