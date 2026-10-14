package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type checkoutRouteGate struct {
	*sqliterepo.Repository
	remaining     atomic.Int32
	read, release chan struct{}
}

func (g *checkoutRouteGate) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	row, err := g.Repository.GetRepository(ctx, id)
	if err != nil || g.remaining.Load() <= 0 || g.remaining.Add(-1) != 0 {
		return row, err
	}
	close(g.read)
	select {
	case <-g.release:
		return row, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type checkoutRouteEvents struct {
	bus.EventBus
	mu     sync.Mutex
	values []*bus.Event
}

func (b *checkoutRouteEvents) Publish(ctx context.Context, subject string, event *bus.Event) error {
	b.mu.Lock()
	b.values = append(b.values, event)
	b.mu.Unlock()
	return b.EventBus.Publish(ctx, subject, event)
}
func (b *checkoutRouteEvents) records() []*bus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]*bus.Event(nil), b.values...)
}

type checkoutRouteFixture struct {
	repo     *sqliterepo.Repository
	gate     *checkoutRouteGate
	api      *service.Service
	router   *gin.Engine
	dispatch *ws.Dispatcher
	events   *checkoutRouteEvents
	db       *sqlx.DB
}

func checkoutRoutePair(t *testing.T) [2]*checkoutRouteFixture {
	t.Helper()
	first, path := workflowHandlerTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(path)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	var f [2]*checkoutRouteFixture
	for i, db := range []*sqlx.DB{first, second} {
		db.SetMaxOpenConns(1)
		store := sqliterepo.NewWithInitializedDB(db, db, nil)
		log := newTestLogger(t)
		memory := bus.NewMemoryEventBus(log)
		t.Cleanup(memory.Close)
		events := &checkoutRouteEvents{EventBus: memory}
		gate := &checkoutRouteGate{Repository: store, read: make(chan struct{}), release: make(chan struct{})}
		api := service.NewService(service.Repos{Workspaces: store, RepoEntities: gate}, events, log, service.RepositoryDiscoveryConfig{})
		f[i] = &checkoutRouteFixture{repo: store, gate: gate, api: api, router: gin.New(), dispatch: ws.NewDispatcher(), events: events, db: db}
		RegisterRepositoryRoutes(f[i].router, f[i].dispatch, api, log)
	}
	require.NoError(t, f[0].repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "route-ws", Name: "Workspace", OwnerID: "route-owner"}))
	require.NoError(t, f[0].repo.CreateRepository(t.Context(), &models.Repository{ID: "route-repo", WorkspaceID: "route-ws", Name: "Before", DefaultBranch: "main", PullBeforeWorktree: true}))
	return f
}
func (f *checkoutRouteFixture) update(t *testing.T, ctx context.Context, transport, payload string) (dto.RepositoryDTO, int) {
	t.Helper()
	var row dto.RepositoryDTO
	if transport == "HTTP" {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/repositories/route-repo", strings.NewReader(payload)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		f.router.ServeHTTP(recorder, req)
		if recorder.Code == 200 {
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &row))
		}
		return row, recorder.Code
	}
	var values map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &values))
	values["id"] = "route-repo"
	request, err := ws.NewRequest("checkout-save", ws.ActionRepositoryUpdate, values)
	require.NoError(t, err)
	response, err := f.dispatch.Dispatch(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, response)
	if response.Type != ws.MessageTypeResponse {
		return row, 400
	}
	require.NoError(t, json.Unmarshal(response.Payload, &row))
	return row, 200
}
func checkoutRouteAssertEvent(t *testing.T, f *checkoutRouteFixture, row dto.RepositoryDTO) {
	t.Helper()
	record := f.events.records()
	require.Len(t, record, 1)
	require.Equal(t, "repository.updated", record[0].Type)
	values := record[0].Data.(map[string]interface{})
	require.Equal(t, row.DefaultBranch, values["default_branch"])
	require.Equal(t, row.PullBeforeWorktree, values["pull_before_worktree"])
}
func checkoutRegisteredFlow(t *testing.T, transport string) {
	t.Helper()
	f := checkoutRoutePair(t)
	ctx, cancel := context.WithTimeout(authn.WithIdentity(t.Context(), authn.Identity{UserID: "route-owner"}), 15*time.Second)
	gate := f[0].gate
	gate.remaining.Store(2)
	var workers sync.WaitGroup
	var release sync.Once
	defer func() { release.Do(func() { close(gate.release) }); cancel(); workers.Wait() }()
	type response struct {
		row  dto.RepositoryDTO
		code int
	}
	done := make(chan response, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		row, code := f[0].update(t, ctx, transport, `{"name":"Saved","default_branch":null}`)
		done <- response{row, code}
	}()
	select {
	case <-gate.read:
	case <-ctx.Done():
		t.Fatal("service snapshot not reached", ctx.Err())
	}
	branch, pull := "develop", false
	_, err := f[1].api.UpdateRepository(ctx, "route-repo", &service.UpdateRepositoryRequest{DefaultBranch: &branch, PullBeforeWorktree: &pull})
	require.NoError(t, err)
	release.Do(func() { close(gate.release) })
	select {
	case result := <-done:
		require.Equal(t, 200, result.code)
		require.Equal(t, "develop", result.row.DefaultBranch)
		require.False(t, result.row.PullBeforeWorktree)
		checkoutRouteAssertEvent(t, f[0], result.row)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stored, err := f[1].repo.GetRepository(ctx, "route-repo")
	require.NoError(t, err)
	require.Equal(t, "develop", stored.DefaultBranch)
	require.False(t, stored.PullBeforeWorktree)
	require.Equal(t, "Saved", stored.Name)
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
func TestRegisteredRepositoryCheckoutDefaultsHTTP(t *testing.T) {
	t.Run("causal_saved_response", func(t *testing.T) { checkoutRegisteredFlow(t, "HTTP") })
	t.Run("explicit_blank_false_null", func(t *testing.T) {
		f := checkoutRoutePair(t)
		row, code := f[0].update(t, t.Context(), "HTTP", `{"default_branch":"","pull_before_worktree":false}`)
		require.Equal(t, 200, code)
		require.Empty(t, row.DefaultBranch)
		require.False(t, row.PullBeforeWorktree)
		row, code = f[0].update(t, t.Context(), "HTTP", `{"default_branch":null,"pull_before_worktree":null}`)
		require.Equal(t, 200, code)
		require.Empty(t, row.DefaultBranch)
		require.False(t, row.PullBeforeWorktree)
	})
	t.Run("atomic_binding_response_and_failure", func(t *testing.T) {
		f := checkoutRoutePair(t)
		crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
		require.NoError(t, err)
		catalog, closeStore, err := secrets.Provide(f[0].db, f[0].db, crypto)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closeStore()) })
		f[0].api.SetSecretStore(catalog)
		secret := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "route-secret", Scope: secrets.ScopeWorkspace, WorkspaceID: "route-ws"}, Value: "not-in-response"}
		require.NoError(t, catalog.Create(t.Context(), secret))
		payload, err := json.Marshal(map[string]any{"default_branch": "binding", "pull_before_worktree": false, "secret_bindings": []map[string]string{{"key": "TOKEN", "secret_id": secret.ID}}})
		require.NoError(t, err)
		row, code := f[0].update(t, t.Context(), "HTTP", string(payload))
		require.Equal(t, 200, code)
		require.Equal(t, "binding", row.DefaultBranch)
		require.False(t, row.PullBeforeWorktree)
		checkoutRouteAssertEvent(t, f[0], row)
		before, err := f[0].repo.GetRepository(t.Context(), "route-repo")
		require.NoError(t, err)
		_, err = f[0].db.Exec(`CREATE TRIGGER reject_route_binding BEFORE INSERT ON repository_secret_bindings BEGIN SELECT RAISE(ABORT, 'route binding rejected'); END`)
		require.NoError(t, err)
		_, code = f[0].update(t, t.Context(), "HTTP", string(payload))
		require.NotEqual(t, 200, code)
		after, err := f[0].repo.GetRepository(t.Context(), "route-repo")
		require.NoError(t, err)
		require.Equal(t, before, after)
		require.Len(t, f[0].events.records(), 1)
	})
	t.Run("admission", func(t *testing.T) { checkoutRouteAdmission(t, "HTTP") })
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
func TestRegisteredRepositoryCheckoutDefaultsWS(t *testing.T) {
	t.Run("causal_saved_response", func(t *testing.T) { checkoutRegisteredFlow(t, "WS") })
	t.Run("branch_schema_only", func(t *testing.T) {
		f := checkoutRoutePair(t)
		row, code := f[0].update(t, t.Context(), "WS", `{"default_branch":"","pull_before_worktree":false}`)
		require.Equal(t, 200, code)
		require.Empty(t, row.DefaultBranch)
		require.True(t, row.PullBeforeWorktree)
	})
	t.Run("admission", func(t *testing.T) { checkoutRouteAdmission(t, "WS") })
}
func checkoutRouteAdmission(t *testing.T, transport string) {
	for _, kind := range []string{"invalid", "foreign", "read_only"} {
		t.Run(kind, func(t *testing.T) {
			f := checkoutRoutePair(t)
			ctx := t.Context()
			payload := `{"default_branch":"bad..branch"}`
			if kind == "foreign" {
				ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "foreign"})
				payload = `{"default_branch":"rejected"}`
			}
			if kind == "read_only" {
				workspace, err := f[0].repo.GetWorkspace(ctx, "route-ws")
				require.NoError(t, err)
				workspace.Name = models.WorkspaceNameImproveKandev
				require.NoError(t, f[0].repo.UpdateWorkspace(ctx, workspace))
				payload = `{"default_branch":"rejected"}`
			}
			_, code := f[0].update(t, ctx, transport, payload)
			require.NotEqual(t, 200, code)
			require.Empty(t, f[0].events.records())
			stored, err := f[0].repo.GetRepository(t.Context(), "route-repo")
			require.NoError(t, err)
			require.Equal(t, "main", stored.DefaultBranch)
			require.True(t, stored.PullBeforeWorktree)
		})
	}
}

func (m *mockRepository) UpdateRepositoryWithCheckoutIntent(ctx context.Context, row *models.Repository, _ repository.RepositoryCheckoutIntent) error {
	return m.UpdateRepository(ctx, row)
}
