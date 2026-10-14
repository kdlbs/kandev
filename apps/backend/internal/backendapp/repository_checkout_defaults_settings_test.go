package backendapp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/settingscatalog"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type checkoutSettingsGate struct {
	*tasksqlite.Repository
	remaining int
	after     func()
}

func (g *checkoutSettingsGate) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	row, err := g.Repository.GetRepository(ctx, id)
	if err == nil && g.remaining > 0 {
		g.remaining--
		if g.remaining == 0 && g.after != nil {
			g.after()
		}
	}
	return row, err
}

type checkoutSettingsEvents struct {
	bus.EventBus
	mu   sync.Mutex
	rows []*bus.Event
}

func (b *checkoutSettingsEvents) Publish(ctx context.Context, subject string, event *bus.Event) error {
	b.mu.Lock()
	b.rows = append(b.rows, event)
	b.mu.Unlock()
	return b.EventBus.Publish(ctx, subject, event)
}

type checkoutSettingsFixture struct {
	api, other *taskservice.Service
	store      *tasksqlite.Repository
	db         *sqlx.DB
	gate       *checkoutSettingsGate
	events     *checkoutSettingsEvents
	dispatch   *ws.Dispatcher
}

func checkoutSettingsHarness(t *testing.T) *checkoutSettingsFixture {
	t.Helper()
	file := filepath.Join(t.TempDir(), "checkout.db")
	raw, err := internaldb.OpenSQLite(file)
	require.NoError(t, err)
	first := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, first.Close()) })
	first.SetMaxOpenConns(1)
	store, err := tasksqlite.NewWithDB(first, first, nil)
	require.NoError(t, err)
	raw, err = internaldb.OpenSQLite(file)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	second.SetMaxOpenConns(1)
	otherStore := tasksqlite.NewWithInitializedDB(second, second, nil)
	log := newTestLogger()
	memory := bus.NewMemoryEventBus(log)
	t.Cleanup(memory.Close)
	events := &checkoutSettingsEvents{EventBus: memory}
	gate := &checkoutSettingsGate{Repository: store}
	api := taskservice.NewService(taskservice.Repos{Workspaces: store, RepoEntities: gate}, events, log, taskservice.RepositoryDiscoveryConfig{})
	other := taskservice.NewService(taskservice.Repos{Workspaces: otherStore, RepoEntities: otherStore}, nil, log, taskservice.RepositoryDiscoveryConfig{})
	require.NoError(t, store.CreateWorkspace(t.Context(), &models.Workspace{ID: "settings-ws", Name: "Workspace", OwnerID: "settings-owner"}))
	require.NoError(t, store.CreateRepository(t.Context(), &models.Repository{ID: "settings-repo", WorkspaceID: "settings-ws", Name: "Before", DefaultBranch: "main", PullBeforeWorktree: true}))
	registry, err := settingscatalog.DefaultRegistry()
	require.NoError(t, err)
	operations := &settingsOperations{registry: registry, deps: settingsDomainDependencies{task: api}}
	h := mcphandlers.NewHandlers(api, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)
	h.SetSettingsCatalog(registry)
	h.SetSettingsOperations(operations)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	return &checkoutSettingsFixture{api: api, other: other, store: store, db: first, gate: gate, events: events, dispatch: dispatcher}
}
func checkoutSettingsPtr[T any](v T) *T { return &v }
func checkoutSettingsContext(ctx context.Context) context.Context {
	return authn.WithIdentity(ctx, authn.Identity{UserID: "settings-owner"})
}
func (f *checkoutSettingsFixture) save(t *testing.T, ctx context.Context, changes map[string]any, workspace string) *ws.Message {
	t.Helper()
	request, err := ws.NewRequest("settings-save", ws.ActionMCPUpdateSettings, map[string]any{"target": map[string]any{"resource_type": "repository", "resource_id": "settings-repo", "workspace_id": workspace}, "changes": changes})
	require.NoError(t, err)
	response, err := f.dispatch.Dispatch(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22, AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
func TestRepositoryCheckoutDefaultsGuardedMCP(t *testing.T) {
	t.Run("causal_compact_result_and_publisher", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		ctx := checkoutSettingsContext(t.Context())
		f.gate.remaining = 2
		f.gate.after = func() {
			_, err := f.other.UpdateRepository(ctx, "settings-repo", &taskservice.UpdateRepositoryRequest{DefaultBranch: checkoutSettingsPtr("develop"), PullBeforeWorktree: checkoutSettingsPtr(false)})
			require.NoError(t, err)
		}
		response := f.save(t, ctx, map[string]any{"repository.name": "Saved"}, "settings-ws")
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		require.Zero(t, f.gate.remaining)
		var payload struct {
			Settings struct {
				DefaultBranch string `json:"default_branch"`
				Pull          bool   `json:"pull_before_worktree"`
				Name          string `json:"name"`
			} `json:"settings"`
		}
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, "develop", payload.Settings.DefaultBranch)
		require.False(t, payload.Settings.Pull)
		require.Equal(t, "Saved", payload.Settings.Name)
		stored, err := f.store.GetRepository(ctx, "settings-repo")
		require.NoError(t, err)
		require.Equal(t, "develop", stored.DefaultBranch)
		require.False(t, stored.PullBeforeWorktree)
		f.events.mu.Lock()
		defer f.events.mu.Unlock()
		require.Len(t, f.events.rows, 1)
		require.Equal(t, "repository.updated", f.events.rows[0].Type)
		values := f.events.rows[0].Data.(map[string]interface{})
		require.Equal(t, "develop", values["default_branch"])
		require.Equal(t, false, values["pull_before_worktree"])
	})
	t.Run("explicit_pair_and_redaction", func(t *testing.T) {
		f := checkoutSettingsHarness(t)
		key, err := secrets.NewMasterKeyProvider(t.TempDir())
		require.NoError(t, err)
		catalog, closeCatalog, err := secrets.Provide(f.db, f.db, key)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closeCatalog()) })
		secret := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "Settings token", Scope: secrets.ScopeWorkspace, WorkspaceID: "settings-ws"}, Value: "disposable-sensitive-value"}
		require.NoError(t, catalog.Create(t.Context(), secret))
		f.api.SetSecretStore(catalog)
		response := f.save(t, checkoutSettingsContext(t.Context()), map[string]any{"default_branch": "", "pull_before_worktree": false, "secret_bindings": []any{map[string]any{"key": "TOKEN", "secret_id": secret.ID}}}, "settings-ws")
		require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		settings := payload["settings"].(map[string]any)
		require.Equal(t, "", settings["default_branch"])
		require.Equal(t, false, settings["pull_before_worktree"])
		binding := settings["secret_bindings"].([]any)[0].(map[string]any)
		require.Equal(t, "TOKEN", binding["key"])
		require.Equal(t, secret.ID, binding["secret_id"])
		require.NotContains(t, binding, "value")
		require.NotContains(t, string(response.Payload), secret.Value)
	})
	for _, kind := range []string{"null", "wrong_workspace", "caller_absent", "foreign_caller", "coordinator_guard"} {
		t.Run(kind, func(t *testing.T) {
			f := checkoutSettingsHarness(t)
			ctx := checkoutSettingsContext(t.Context())
			changes := map[string]any{"pull_before_worktree": false}
			workspace := "settings-ws"
			switch kind {
			case "null":
				changes["pull_before_worktree"] = nil
			case "wrong_workspace":
				workspace = "foreign"
			case "caller_absent":
				ctx = t.Context()
			case "foreign_caller":
				ctx = authn.WithIdentity(t.Context(), authn.Identity{UserID: "foreign"})
			case "coordinator_guard":
				ctx = mcpscope.WithPrincipal(ctx, mcpscope.Principal{WorkspaceID: "settings-ws", CallerTaskID: "task", CallerSessionID: "session", CoordinatorID: "coord", Surface: mcpprofile.SurfaceCoordinator})
			}
			response := f.save(t, ctx, changes, workspace)
			require.Equal(t, ws.MessageTypeError, response.Type)
			stored, err := f.store.GetRepository(t.Context(), "settings-repo")
			require.NoError(t, err)
			require.True(t, stored.PullBeforeWorktree)
			require.Empty(t, f.events.rows)
		})
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
func TestRepositoryCheckoutDefaultsExactHost(t *testing.T) {
	f := checkoutSettingsHarness(t)
	ctx := checkoutSettingsContext(t.Context())
	adapter := pluginsWorkspaceAdminAdapter{tasks: f.api}
	before, err := f.store.GetRepository(ctx, "settings-repo")
	require.NoError(t, err)
	version := before.UpdatedAt.UTC().Format(time.RFC3339Nano)
	input := &pluginsdk.WorkspaceRepositoryUpdate{RepositoryID: before.ID, ExpectedResourceVersion: version, DefaultBranch: checkoutSettingsPtr("exact"), PullBeforeWorktree: checkoutSettingsPtr(false)}
	result, err := adapter.updateRepository(ctx, "settings-ws", input)
	require.NoError(t, err)
	require.NotEmpty(t, result.ResourceVersion)
	replay, err := adapter.updateRepository(ctx, "settings-ws", input)
	require.NoError(t, err)
	require.True(t, replay.AlreadyApplied)
	current, err := f.store.GetRepository(ctx, before.ID)
	require.NoError(t, err)
	version = current.UpdatedAt.UTC().Format(time.RFC3339Nano)
	_, err = f.other.UpdateRepository(ctx, before.ID, &taskservice.UpdateRepositoryRequest{Name: checkoutSettingsPtr("Disjoint")})
	require.NoError(t, err)
	_, err = adapter.updateRepository(ctx, "settings-ws", &pluginsdk.WorkspaceRepositoryUpdate{RepositoryID: before.ID, ExpectedResourceVersion: version, DefaultBranch: checkoutSettingsPtr("rejected")})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
	_, err = adapter.updateRepository(ctx, "foreign", input)
	require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
	stored, err := f.store.GetRepository(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "exact", stored.DefaultBranch)
	require.False(t, stored.PullBeforeWorktree)
	require.Len(t, f.events.rows, 1)
}
