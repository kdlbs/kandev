package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type pgCheckoutPublisher struct {
	bus.EventBus
	mu   sync.Mutex
	rows []*bus.Event
}

func (p *pgCheckoutPublisher) Publish(ctx context.Context, subject string, event *bus.Event) error {
	p.mu.Lock()
	p.rows = append(p.rows, event)
	p.mu.Unlock()
	return p.EventBus.Publish(ctx, subject, event)
}
func (p *pgCheckoutPublisher) pair(t *testing.T, branch string, pull bool) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	require.Len(t, p.rows, 1)
	require.Equal(t, "repository.updated", p.rows[0].Type)
	values := p.rows[0].Data.(map[string]interface{})
	require.Equal(t, branch, values["default_branch"])
	require.Equal(t, pull, values["pull_before_worktree"])
}
func pgCheckoutAPI(t *testing.T, repo *tasksqlite.Repository) (*service.Service, *pgCheckoutPublisher) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	memory := bus.NewMemoryEventBus(log)
	t.Cleanup(memory.Close)
	publisher := &pgCheckoutPublisher{EventBus: memory}
	return service.NewService(service.Repos{Workspaces: repo, RepoEntities: repo}, publisher, log, service.RepositoryDiscoveryConfig{}), publisher
}
func pgCheckoutSeed(t *testing.T) (*tasksqlite.Repository, *tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	a, b, observer := workflowPostgresPair(t)
	require.NoError(t, a.CreateWorkspace(t.Context(), &models.Workspace{ID: "pg-checkout-ws", Name: "Workspace"}))
	require.NoError(t, a.CreateRepository(t.Context(), &models.Repository{ID: "pg-checkout", WorkspaceID: "pg-checkout-ws", Name: "Before", DefaultBranch: "main", PullBeforeWorktree: true}))
	return a, b, observer
}
func pgCheckoutPtr[T any](value T) *T { return &value }

func checkoutSeedDisabled(t *testing.T, store *tasksqlite.Repository) {
	t.Helper()
	row, err := store.GetRepository(t.Context(), "pg-checkout")
	require.NoError(t, err)
	row.PullBeforeWorktree = false
	require.NoError(t, store.UpdateRepository(t.Context(), row))
	seeded, err := store.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.False(t, seeded.PullBeforeWorktree)
}

func checkoutPostgresPID(t *testing.T, database *sql.DB) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var pid int
	require.NoError(t, database.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	return pid
}

type pgCheckoutReadGate struct {
	*tasksqlite.Repository
	after func()
}

func (g *pgCheckoutReadGate) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	row, err := g.Repository.GetRepository(ctx, id)
	if err == nil && g.after != nil {
		after := g.after
		g.after = nil
		after()
	}
	return row, err
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
func TestRepositoryCheckoutDefaultsPhysicalPostgres(t *testing.T) {
	for _, enable := range []bool{false, true} {
		for held := range 2 {
			t.Run(fmt.Sprintf("stale_snapshot/enable_%t/held_%d", enable, held), func(t *testing.T) {
				a, b, observer := pgCheckoutSeed(t)
				if enable {
					checkoutSeedDisabled(t, a)
				}
				require.NotEqual(t, checkoutPostgresPID(t, a.DB()), checkoutPostgresPID(t, b.DB()))
				require.NotEqual(t, checkoutPostgresPID(t, b.DB()), checkoutPostgresPID(t, observer.DB))
				_, firstEvents := pgCheckoutAPI(t, a)
				second, _ := pgCheckoutAPI(t, b)
				gate := &pgCheckoutReadGate{Repository: a}
				first := service.NewService(service.Repos{Workspaces: a, RepoEntities: gate}, firstEvents, newCheckoutPGLogger(t), service.RepositoryDiscoveryConfig{})
				requests := [2]*service.UpdateRepositoryRequest{{DefaultBranch: pgCheckoutPtr("develop")}, {PullBeforeWorktree: &enable}}
				gate.after = func() {
					_, err := second.UpdateRepository(t.Context(), "pg-checkout", requests[1-held])
					require.NoError(t, err)
				}
				row, err := first.UpdateRepository(t.Context(), "pg-checkout", requests[held])
				require.NoError(t, err)
				require.Equal(t, "develop", row.DefaultBranch)
				require.Equal(t, enable, row.PullBeforeWorktree)
				firstEvents.pair(t, "develop", enable)
				stored, err := b.GetRepository(t.Context(), row.ID)
				require.NoError(t, err)
				require.Equal(t, "develop", stored.DefaultBranch)
				require.Equal(t, enable, stored.PullBeforeWorktree)
			})
		}
	}
	for _, kind := range []string{"branch_rename", "enable_rename", "disable_rename", "both_rename", "branch_omitted_pull", "pull_omitted_branch", "explicit_both", "rename_holder"} {
		t.Run("actual_wait/"+kind, func(t *testing.T) {
			a, b, observer := pgCheckoutSeed(t)
			if kind == "enable_rename" {
				checkoutSeedDisabled(t, a)
			}
			api, publisher := pgCheckoutAPI(t, b)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			holderPID, writerPID := checkoutPostgresPID(t, a.DB()), checkoutPostgresPID(t, b.DB())
			observerPID := checkoutPostgresPID(t, observer.DB)
			require.NotEqual(t, holderPID, writerPID)
			require.NotEqual(t, holderPID, observerPID)
			require.NotEqual(t, writerPID, observerPID)
			tx, err := a.DB().BeginTx(ctx, nil)
			require.NoError(t, err)
			var workers sync.WaitGroup
			defer func() { _ = tx.Rollback(); cancel(); workers.Wait() }()
			_, err = tx.ExecContext(ctx, `SELECT id FROM repositories WHERE id='pg-checkout' FOR UPDATE`)
			require.NoError(t, err)
			request := &service.UpdateRepositoryRequest{Name: pgCheckoutPtr("Saved")}
			branch, pull := "develop", false
			holderSQL := `UPDATE repositories SET default_branch=$1,pull_before_worktree=$2 WHERE id='pg-checkout'`
			args := []any{branch, 0}
			switch kind {
			case "branch_rename":
				pull = true
				args = []any{branch, 1}
			case "enable_rename":
				branch = "main"
				pull = true
				args = []any{branch, 1}
			case "disable_rename":
				branch = "main"
				args = []any{branch, 0}
			case "branch_omitted_pull":
				request = &service.UpdateRepositoryRequest{DefaultBranch: pgCheckoutPtr("release")}
				branch = "release"
			case "pull_omitted_branch":
				request = &service.UpdateRepositoryRequest{PullBeforeWorktree: pgCheckoutPtr(false)}
			case "explicit_both":
				request = &service.UpdateRepositoryRequest{DefaultBranch: pgCheckoutPtr("release"), PullBeforeWorktree: pgCheckoutPtr(true)}
				branch = "release"
				pull = true
			case "rename_holder":
				holderSQL = `UPDATE repositories SET name='Holder rename' WHERE id='pg-checkout'`
				args = nil
				request = &service.UpdateRepositoryRequest{DefaultBranch: pgCheckoutPtr("develop")}
				pull = true
			}
			done := make(chan struct {
				row *models.Repository
				err error
			}, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				row, err := api.UpdateRepository(ctx, "pg-checkout", request)
				done <- struct {
					row *models.Repository
					err error
				}{row, err}
			}()
			waitSettingsPostgresWriter(t, ctx, observer, holderPID, writerPID)
			t.Logf("physically observed holder=%d writer=%d observer=%d", holderPID, writerPID, observerPID)
			_, err = tx.ExecContext(ctx, holderSQL, args...)
			require.NoError(t, err)
			require.NoError(t, tx.Commit())
			select {
			case result := <-done:
				require.NoError(t, result.err)
				require.Equal(t, branch, result.row.DefaultBranch)
				require.Equal(t, pull, result.row.PullBeforeWorktree)
				publisher.pair(t, branch, pull)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			stored, err := a.GetRepository(ctx, "pg-checkout")
			require.NoError(t, err)
			require.Equal(t, branch, stored.DefaultBranch)
			require.Equal(t, pull, stored.PullBeforeWorktree)
		})
	}
}
func newCheckoutPGLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	return log
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
func TestRepositoryCheckoutDefaultsPostgresFailures(t *testing.T) {
	for _, kind := range []string{"rollback", "delete", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			a, b, observer := pgCheckoutSeed(t)
			api, publisher := pgCheckoutAPI(t, b)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			holderPID, writerPID := checkoutPostgresPID(t, a.DB()), checkoutPostgresPID(t, b.DB())
			observerPID := checkoutPostgresPID(t, observer.DB)
			require.NotEqual(t, holderPID, writerPID)
			require.NotEqual(t, holderPID, observerPID)
			require.NotEqual(t, writerPID, observerPID)
			tx, err := a.DB().BeginTx(t.Context(), nil)
			require.NoError(t, err)
			var workers sync.WaitGroup
			defer func() { _ = tx.Rollback(); cancel(); workers.Wait() }()
			_, err = tx.ExecContext(ctx, `SELECT id FROM repositories WHERE id='pg-checkout' FOR UPDATE`)
			require.NoError(t, err)
			done := make(chan error, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				_, err := api.UpdateRepository(ctx, "pg-checkout", &service.UpdateRepositoryRequest{Name: pgCheckoutPtr("Saved")})
				done <- err
			}()
			waitSettingsPostgresWriter(t, ctx, observer, holderPID, writerPID)
			switch kind {
			case "rollback":
				_, err = tx.ExecContext(ctx, `UPDATE repositories SET default_branch='uncommitted',pull_before_worktree=0 WHERE id='pg-checkout'`)
				require.NoError(t, err)
				require.NoError(t, tx.Rollback())
			case "delete":
				_, err = tx.ExecContext(ctx, `UPDATE repositories SET deleted_at=NOW() WHERE id='pg-checkout'`)
				require.NoError(t, err)
				require.NoError(t, tx.Commit())
			case "cancel":
				cancel()
			}
			select {
			case writeErr := <-done:
				if kind == "rollback" {
					require.NoError(t, writeErr)
					publisher.pair(t, "main", true)
				} else {
					require.Error(t, writeErr)
					require.Empty(t, publisher.rows)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("writer did not join")
			}
			if kind == "cancel" {
				require.NoError(t, tx.Rollback())
			}
		})
	}
	t.Run("companion_exact_and_cas", func(t *testing.T) {
		a, _, _ := pgCheckoutSeed(t)
		row, err := a.GetRepository(t.Context(), "pg-checkout")
		require.NoError(t, err)
		before := *row
		bindings := []models.RepositorySecretBinding{{Key: "DUP", SecretID: "ref"}, {Key: "DUP", SecretID: "ref"}}
		require.Error(t, a.UpdateRepositoryWithSecretBindingsAndCheckoutIntent(t.Context(), row, bindings, models.RepositoryCheckoutIntent{DefaultBranch: pgCheckoutPtr("rejected"), PullBeforeWorktree: pgCheckoutPtr(false)}))
		after, err := a.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		require.Equal(t, &before, after)
		version := after.UpdatedAt
		require.NoError(t, a.UpdateRepositoryWithCheckoutIntent(t.Context(), after, models.RepositoryCheckoutIntent{}))
		require.ErrorIs(t, a.UpdateRepositoryIfUnchanged(t.Context(), row, version), repoerrors.ErrTaskVersionConflict)
		require.ErrorIs(t, a.UpdateRepositoryWithSecretBindingsIfUnchanged(t.Context(), row, nil, version), repoerrors.ErrTaskVersionConflict)
		fresh, err := a.GetRepository(t.Context(), row.ID)
		require.NoError(t, err)
		fresh.DefaultBranch = "exact"
		fresh.PullBeforeWorktree = false
		require.NoError(t, a.UpdateRepositoryWithSecretBindingsIfUnchanged(t.Context(), fresh, nil, fresh.UpdatedAt))
		require.NoError(t, a.UpdateRepositoryDefaultBranch(t.Context(), fresh.ID, "exact", "recovered"))
		require.Error(t, a.UpdateRepositoryDefaultBranch(t.Context(), fresh.ID, "exact", "wrong"))
		stored, err := a.GetRepository(t.Context(), fresh.ID)
		require.NoError(t, err)
		require.False(t, stored.PullBeforeWorktree)
		require.Equal(t, "recovered", stored.DefaultBranch)
		stored.DefaultBranch = "legacy"
		stored.PullBeforeWorktree = true
		require.NoError(t, a.UpdateRepository(t.Context(), stored))
		loaded, err := a.GetRepository(t.Context(), stored.ID)
		require.NoError(t, err)
		require.Equal(t, "legacy", loaded.DefaultBranch)
		require.True(t, loaded.PullBeforeWorktree)
	})
}
