package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type checkoutReadGate struct {
	*sqliterepo.Repository
	armed    atomic.Bool
	observed chan struct{}
	release  chan struct{}
}

func (g *checkoutReadGate) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	row, err := g.Repository.GetRepository(ctx, id)
	if err != nil || !g.armed.Swap(false) {
		return row, err
	}
	close(g.observed)
	select {
	case <-g.release:
		return row, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type checkoutServiceFixture struct {
	api  *Service
	gate *checkoutReadGate
	bus  *MockEventBus
	db   *sqlx.DB
}

func checkoutPtr[T any](v T) *T { return &v }
func checkoutServices(t *testing.T, pull bool) [2]checkoutServiceFixture {
	t.Helper()
	first, path := serviceTestSQLiteTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(path)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	var fixtures [2]checkoutServiceFixture
	var drivers [2]any
	for i, db := range []*sqlx.DB{first, second} {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		conn, err := db.Conn(t.Context())
		require.NoError(t, err)
		require.NoError(t, conn.Raw(func(v any) error { drivers[i] = v; return nil }))
		require.NoError(t, conn.Close())
		store := sqliterepo.NewWithInitializedDB(db, db, nil)
		gate := &checkoutReadGate{Repository: store, observed: make(chan struct{}), release: make(chan struct{})}
		events := NewMockEventBus()
		t.Cleanup(events.Close)
		fixtures[i] = checkoutServiceFixture{api: NewService(Repos{Workspaces: store, RepoEntities: gate}, events, accessTestLogger(t), RepositoryDiscoveryConfig{}), gate: gate, bus: events, db: db}
	}
	require.NotEqual(t, fmt.Sprintf("%p", drivers[0]), fmt.Sprintf("%p", drivers[1]))
	t.Logf("independent SQLite physical handles %p / %p", drivers[0], drivers[1])
	require.NoError(t, fixtures[0].gate.CreateWorkspace(t.Context(), &models.Workspace{ID: "checkout-ws", Name: "Workspace"}))
	require.NoError(t, fixtures[0].gate.CreateRepository(t.Context(), &models.Repository{ID: "checkout-repo", WorkspaceID: "checkout-ws", Name: "Before", DefaultBranch: "main", PullBeforeWorktree: pull}))
	return fixtures
}

type checkoutOutcome struct {
	row *models.Repository
	err error
}

func checkoutOverlap(t *testing.T, f [2]checkoutServiceFixture, held int, requests [2]*UpdateRepositoryRequest) [2]checkoutOutcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	gate := f[held].gate
	var joined sync.WaitGroup
	var released sync.Once
	defer func() { released.Do(func() { close(gate.release) }); cancel(); joined.Wait() }()
	gate.armed.Store(true)
	done := make(chan checkoutOutcome, 1)
	joined.Add(1)
	go func() {
		defer joined.Done()
		row, err := f[held].api.UpdateRepository(ctx, "checkout-repo", requests[held])
		done <- checkoutOutcome{row, err}
	}()
	select {
	case <-gate.observed:
	case <-ctx.Done():
		t.Fatal("real snapshot read not observed", ctx.Err())
	}
	var result [2]checkoutOutcome
	result[1-held].row, result[1-held].err = f[1-held].api.UpdateRepository(ctx, "checkout-repo", requests[1-held])
	require.NoError(t, result[1-held].err)
	released.Do(func() { close(gate.release) })
	select {
	case result[held] = <-done:
	case <-ctx.Done():
		t.Fatal("held real service not joined", ctx.Err())
	}
	return result
}
func checkoutAssertEvent(t *testing.T, f checkoutServiceFixture, row *models.Repository) {
	t.Helper()
	published := f.bus.GetPublishedEvents()
	require.Len(t, published, 1)
	require.Equal(t, "repository.updated", published[0].Type)
	data, ok := published[0].Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, row.DefaultBranch, data["default_branch"])
	assert.Equal(t, row.PullBeforeWorktree, data["pull_before_worktree"])
	assert.Equal(t, row.UpdatedAt.Format(time.RFC3339), data["updated_at"])
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
func TestRepositoryCheckoutDefaultsConcurrentSQLite(t *testing.T) {
	cases := []struct {
		name        string
		initial     bool
		left, right *UpdateRepositoryRequest
		branch      string
		pull        bool
	}{
		{"branch_rename", false, &UpdateRepositoryRequest{Name: checkoutPtr("Saved")}, &UpdateRepositoryRequest{DefaultBranch: checkoutPtr("develop")}, "develop", false},
		{"enable_rename", false, &UpdateRepositoryRequest{Name: checkoutPtr("Saved")}, &UpdateRepositoryRequest{PullBeforeWorktree: checkoutPtr(true)}, "main", true},
		{"disable_rename", true, &UpdateRepositoryRequest{Name: checkoutPtr("Saved")}, &UpdateRepositoryRequest{PullBeforeWorktree: checkoutPtr(false)}, "main", false},
		{"both_rename", false, &UpdateRepositoryRequest{Name: checkoutPtr("Saved")}, &UpdateRepositoryRequest{DefaultBranch: checkoutPtr("develop"), PullBeforeWorktree: checkoutPtr(true)}, "develop", true},
		{"branch_with_omitted_pull", false, &UpdateRepositoryRequest{DefaultBranch: checkoutPtr("release")}, &UpdateRepositoryRequest{PullBeforeWorktree: checkoutPtr(true)}, "release", true},
	}
	for _, tc := range cases {
		for held := range 2 {
			t.Run(fmt.Sprintf("%s/held_%d", tc.name, held), func(t *testing.T) {
				f := checkoutServices(t, tc.initial)
				result := checkoutOverlap(t, f, held, [2]*UpdateRepositoryRequest{tc.left, tc.right})
				require.NoError(t, result[held].err)
				stored, err := f[0].gate.Repository.GetRepository(t.Context(), "checkout-repo")
				require.NoError(t, err)
				assert.Equal(t, tc.branch, stored.DefaultBranch, "STORED branch")
				assert.Equal(t, tc.pull, stored.PullBeforeWorktree, "STORED pull")
				assert.Equal(t, tc.branch, result[held].row.DefaultBranch, "RETURNED branch")
				assert.Equal(t, tc.pull, result[held].row.PullBeforeWorktree, "RETURNED pull")
				if held == 0 && tc.left.Name != nil {
					assert.Equal(t, "Saved", stored.Name)
				}
				for i := range f {
					checkoutAssertEvent(t, f[i], result[i].row)
				}
			})
		}
	}
	t.Run("explicit_both_control", func(t *testing.T) {
		f := checkoutServices(t, true)
		result := checkoutOverlap(t, f, 0, [2]*UpdateRepositoryRequest{{DefaultBranch: checkoutPtr("release"), PullBeforeWorktree: checkoutPtr(false)}, {DefaultBranch: checkoutPtr("develop"), PullBeforeWorktree: checkoutPtr(true)}})
		require.NoError(t, result[0].err)
		require.Equal(t, "release", result[0].row.DefaultBranch)
		require.False(t, result[0].row.PullBeforeWorktree)
	})
	t.Run("uncontested_control", func(t *testing.T) {
		f := checkoutServices(t, false)
		row, err := f[0].api.UpdateRepository(t.Context(), "checkout-repo", &UpdateRepositoryRequest{Name: checkoutPtr("Saved")})
		require.NoError(t, err)
		require.Equal(t, "main", row.DefaultBranch)
		require.False(t, row.PullBeforeWorktree)
	})
}
