package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orgunit"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type registeredSettingsSnapshot struct {
	*sqliterepo.Repository
	hold    atomic.Bool
	read    chan struct{}
	release chan struct{}
}

func (r *registeredSettingsSnapshot) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	row, err := r.Repository.GetWorkspace(ctx, id)
	if err != nil || !r.hold.Swap(false) {
		return row, err
	}
	close(r.read)
	select {
	case <-r.release:
		return row, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type registeredSettingsFixture struct {
	repo         *sqliterepo.Repository
	database     *sqlx.DB
	gate         *registeredSettingsSnapshot
	service      *service.Service
	router       *gin.Engine
	dispatch     *ws.Dispatcher
	observations chan *bus.Event
}

func registeredSettingsPair(t *testing.T) [2]*registeredSettingsFixture {
	t.Helper()
	first, filename := workflowHandlerTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(filename)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	var pair [2]*registeredSettingsFixture
	var physical [2]any
	for i, database := range []*sqlx.DB{first, second} {
		database.SetMaxOpenConns(1)
		database.SetMaxIdleConns(1)
		conn, err := database.Conn(t.Context())
		require.NoError(t, err)
		err = conn.Raw(func(driver any) error { physical[i] = driver; return nil })
		closeErr := conn.Close()
		require.NoError(t, err)
		require.NoError(t, closeErr)
		log := newTestLogger(t)
		f := &registeredSettingsFixture{
			repo:     sqliterepo.NewWithInitializedDB(database, database, nil),
			database: database,
			router:   gin.New(), dispatch: ws.NewDispatcher(), observations: make(chan *bus.Event, 16),
		}
		f.gate = &registeredSettingsSnapshot{Repository: f.repo, read: make(chan struct{}), release: make(chan struct{})}
		eventBus := bus.NewMemoryEventBus(log)
		t.Cleanup(eventBus.Close)
		_, err = eventBus.Subscribe(events.WorkspaceUpdated, func(ctx context.Context, event *bus.Event) error {
			select {
			case f.observations <- event:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		require.NoError(t, err)
		f.service = service.NewService(service.Repos{Workspaces: f.gate}, eventBus, log, service.RepositoryDiscoveryConfig{})
		RegisterWorkspaceRoutes(f.router, f.dispatch, f.service, log)
		pair[i] = f
	}
	require.NotEqual(t, fmt.Sprintf("%p", physical[0]), fmt.Sprintf("%p", physical[1]))
	t.Logf("registered settings SQLite connections: %p / %p", physical[0], physical[1])
	executor, environment, agent, config := "kept-executor", "kept-environment", "kept-agent", "kept-config"
	require.NoError(t, pair[0].repo.CreateWorkspace(t.Context(), &models.Workspace{
		ID: "registered-settings", Name: "Before", Description: "Before description", OwnerID: "transport-owner",
		DefaultExecutorID: &executor, DefaultEnvironmentID: &environment, DefaultAgentProfileID: &agent, DefaultConfigAgentProfileID: &config,
		ACPIdleTimeoutMinutes: 120,
	}))
	return pair
}

type registeredSettingsResult struct {
	row    dto.WorkspaceDTO
	status int
	frame  ws.MessageType
	err    error
}

func (f *registeredSettingsFixture) update(ctx context.Context, transport, payload string) registeredSettingsResult {
	var result registeredSettingsResult
	if transport == "HTTP" {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/registered-settings", strings.NewReader(payload)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		f.router.ServeHTTP(recorder, request)
		result.status = recorder.Code
		if result.status == http.StatusOK {
			result.err = json.Unmarshal(recorder.Body.Bytes(), &result.row)
		}
		return result
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		response, dispatchErr := f.dispatch.Dispatch(ctx, &ws.Message{ID: "settings-malformed", Type: ws.MessageTypeRequest, Action: ws.ActionWorkspaceUpdate, Payload: json.RawMessage(payload)})
		result.err = dispatchErr
		if response != nil {
			result.frame = response.Type
		}
		return result
	}
	body["id"] = "registered-settings"
	request, err := ws.NewRequest("settings-request", ws.ActionWorkspaceUpdate, body)
	if err != nil {
		result.err = err
		return result
	}
	response, err := f.dispatch.Dispatch(ctx, request)
	result.err = err
	if response == nil {
		return result
	}
	result.frame = response.Type
	if result.frame == ws.MessageTypeResponse {
		result.err = json.Unmarshal(response.Payload, &result.row)
	}
	return result
}

func requireSettingsSuccess(t *testing.T, transport string, result registeredSettingsResult) {
	t.Helper()
	require.NoError(t, result.err)
	if transport == "HTTP" {
		require.Equal(t, http.StatusOK, result.status)
	} else {
		require.Equal(t, ws.MessageTypeResponse, result.frame)
	}
}

func settingsDTOText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func requireSettingsObservation(t *testing.T, f *registeredSettingsFixture, row dto.WorkspaceDTO) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var event *bus.Event
	select {
	case event = <-f.observations:
	case <-ctx.Done():
		t.Fatal("registered workspace event subscriber did not observe success", ctx.Err())
	}
	require.Equal(t, events.WorkspaceUpdated, event.Type)
	data, ok := event.Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, row.ID, data["id"])
	require.Equal(t, row.Name, data["name"])
	require.Equal(t, settingsDTOText(row.Description), data["description"])
	require.Equal(t, row.UnitID, data["unit_id"])
	require.Equal(t, row.DefaultExecutorID, data["default_executor_id"])
	require.Equal(t, row.DefaultEnvironmentID, data["default_environment_id"])
	require.Equal(t, row.DefaultAgentProfileID, data["default_agent_profile_id"])
	require.Equal(t, row.DefaultConfigAgentProfileID, data["default_config_agent_profile_id"])
	require.Equal(t, row.ACPIdleSuspensionEnabled, data["acp_idle_suspension_enabled"])
	require.Equal(t, row.ACPIdleTimeoutMinutes, data["acp_idle_timeout_minutes"])
	require.Equal(t, row.UpdatedAt.Format(time.RFC3339), data["updated_at"])
	select {
	case extra := <-f.observations:
		t.Fatalf("unexpected extra successful workspace event: %v", extra)
	default:
	}
}

func registeredSettingsContract(t *testing.T, transport string) {
	t.Helper()
	for held := range 2 {
		t.Run(fmt.Sprintf("overlap/held_%d", held), func(t *testing.T) {
			pair := registeredSettingsPair(t)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			gate := pair[held].gate
			var workers sync.WaitGroup
			var release sync.Once
			defer func() { release.Do(func() { close(gate.release) }); cancel(); workers.Wait() }()
			gate.hold.Store(true)
			payloads := [2]string{`{"name":"Transport saved"}`, `{"acp_idle_suspension_enabled":true,"acp_idle_timeout_minutes":45}`}
			done := make(chan registeredSettingsResult, 1)
			workers.Add(1)
			go func() { defer workers.Done(); done <- pair[held].update(ctx, transport, payloads[held]) }()
			select {
			case <-gate.read:
			case <-ctx.Done():
				t.Fatal("registered workspace service snapshot not reached", ctx.Err())
			}
			var results [2]registeredSettingsResult
			results[1-held] = pair[1-held].update(ctx, transport, payloads[1-held])
			requireSettingsSuccess(t, transport, results[1-held])
			release.Do(func() { close(gate.release) })
			select {
			case results[held] = <-done:
			case <-ctx.Done():
				t.Fatal("registered workspace held request did not settle", ctx.Err())
			}
			requireSettingsSuccess(t, transport, results[held])
			stored, err := pair[0].repo.GetWorkspace(ctx, "registered-settings")
			require.NoError(t, err)
			require.Equal(t, "Transport saved", stored.Name)
			require.True(t, stored.ACPIdleSuspensionEnabled)
			require.Equal(t, 45, stored.ACPIdleTimeoutMinutes)
			require.Equal(t, dto.FromWorkspace(stored), results[held].row)
			for i := range pair {
				requireSettingsObservation(t, pair[i], results[i].row)
			}
		})
	}
	t.Run("presence_and_error_controls", func(t *testing.T) {
		pair := registeredSettingsPair(t)
		f := pair[0]
		for _, payload := range []string{`{}`, `{"name":null,"description":null,"default_executor_id":null,"default_environment_id":null,"default_agent_profile_id":null,"default_config_agent_profile_id":null,"acp_idle_suspension_enabled":null,"acp_idle_timeout_minutes":null,"unit_id":null}`} {
			before, err := f.repo.GetWorkspace(t.Context(), "registered-settings")
			require.NoError(t, err)
			result := f.update(t.Context(), transport, payload)
			requireSettingsSuccess(t, transport, result)
			requireSettingsObservation(t, f, result.row)
			stored, err := pair[1].repo.GetWorkspace(t.Context(), before.ID)
			require.NoError(t, err)
			require.True(t, stored.UpdatedAt.After(before.UpdatedAt))
			stored.UpdatedAt = before.UpdatedAt
			require.Equal(t, before, stored)
		}
		for _, payload := range []string{
			`{"name":"","description":"","default_executor_id":"","default_environment_id":"  ","default_agent_profile_id":"  selected-agent  ","default_config_agent_profile_id":" selected-config ","acp_idle_suspension_enabled":true,"acp_idle_timeout_minutes":19}`,
			`{"default_agent_profile_id":"","default_config_agent_profile_id":" ","acp_idle_suspension_enabled":false}`,
		} {
			result := f.update(t.Context(), transport, payload)
			requireSettingsSuccess(t, transport, result)
			requireSettingsObservation(t, f, result.row)
		}
		before, err := f.repo.GetWorkspace(t.Context(), "registered-settings")
		require.NoError(t, err)
		require.Empty(t, before.Name)
		require.Empty(t, before.Description)
		require.Nil(t, before.DefaultExecutorID)
		require.Nil(t, before.DefaultEnvironmentID)
		require.Nil(t, before.DefaultAgentProfileID)
		require.Nil(t, before.DefaultConfigAgentProfileID)
		require.False(t, before.ACPIdleSuspensionEnabled)
		require.Equal(t, 19, before.ACPIdleTimeoutMinutes)
		for _, payload := range []string{`{`, `{"name":42}`, `{"acp_idle_suspension_enabled":"false"}`, `{"acp_idle_timeout_minutes":0}`, `{"acp_idle_timeout_minutes":-7}`} {
			result := f.update(t.Context(), transport, payload)
			require.NoError(t, result.err)
			if transport == "HTTP" {
				require.Equal(t, http.StatusBadRequest, result.status)
			} else {
				require.Equal(t, ws.MessageTypeError, result.frame)
			}
		}
		outsider := authn.WithIdentity(t.Context(), authn.Identity{UserID: "other-user", Role: authn.RoleMember})
		denied := f.update(outsider, transport, `{"name":"Denied"}`)
		require.NoError(t, denied.err)
		if transport == "HTTP" {
			require.Equal(t, http.StatusNotFound, denied.status)
		} else {
			require.Equal(t, ws.MessageTypeError, denied.frame)
		}
		_, err = f.repo.DB().ExecContext(t.Context(), `CREATE TRIGGER reject_registered_settings BEFORE UPDATE ON workspaces BEGIN SELECT RAISE(ABORT, 'registered settings rejected'); END`)
		require.NoError(t, err)
		rejected := f.update(t.Context(), transport, `{"name":"Rejected","acp_idle_suspension_enabled":true}`)
		require.NoError(t, rejected.err)
		if transport == "HTTP" {
			require.Equal(t, http.StatusInternalServerError, rejected.status)
		} else {
			require.Equal(t, ws.MessageTypeError, rejected.frame)
		}
		stored, err := pair[1].repo.GetWorkspace(t.Context(), before.ID)
		require.NoError(t, err)
		require.Equal(t, before, stored)
		select {
		case event := <-f.observations:
			t.Fatalf("rejected update published success: %v", event)
		default:
		}
		_, err = f.repo.DB().ExecContext(t.Context(), `DROP TRIGGER reject_registered_settings`)
		require.NoError(t, err)
		positive := f.update(t.Context(), transport, `{"name":"Positive control"}`)
		requireSettingsSuccess(t, transport, positive)
		requireSettingsObservation(t, f, positive.row)
	})
	t.Run("existing_unit_transport_capability", func(t *testing.T) {
		pair := registeredSettingsPair(t)
		f := pair[0]
		store, err := orgunit.NewStore(internaldb.NewPool(f.database, f.database))
		require.NoError(t, err)
		units := orgunit.NewService(store, nil)
		root, err := units.EnsureRoot(t.Context(), "", "Transport org")
		require.NoError(t, err)
		destination, err := units.Create(t.Context(), root.ID, "Destination")
		require.NoError(t, err)
		require.NoError(t, f.repo.PlaceWorkspace(t.Context(), "registered-settings", root.ID))
		f.service.SetUnitPlacer(units)
		payload, err := json.Marshal(map[string]any{"unit_id": destination.ID, "visibility": "org"})
		require.NoError(t, err)
		result := f.update(t.Context(), transport, string(payload))
		requireSettingsSuccess(t, transport, result)
		want := root.ID
		if transport == "HTTP" {
			want = destination.ID
		}
		require.Equal(t, want, result.row.UnitID)
		stored, err := f.repo.GetWorkspace(t.Context(), "registered-settings")
		require.NoError(t, err)
		require.Equal(t, want, stored.UnitID)
		requireSettingsObservation(t, f, result.row)
	})
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.1, AC-WORKSPACES-SETTINGS-UPDATES-001.2, AC-WORKSPACES-SETTINGS-UPDATES-001.4, AC-WORKSPACES-SETTINGS-UPDATES-001.6, AC-WORKSPACES-SETTINGS-UPDATES-001.7, AC-WORKSPACES-SETTINGS-UPDATES-001.8
func TestRegisteredWorkspaceFieldUpdatesHTTP(t *testing.T) { registeredSettingsContract(t, "HTTP") }

// @covers AC-WORKSPACES-SETTINGS-UPDATES-001.1, AC-WORKSPACES-SETTINGS-UPDATES-001.2, AC-WORKSPACES-SETTINGS-UPDATES-001.4, AC-WORKSPACES-SETTINGS-UPDATES-001.6, AC-WORKSPACES-SETTINGS-UPDATES-001.7, AC-WORKSPACES-SETTINGS-UPDATES-001.8
func TestRegisteredWorkspaceFieldUpdatesWS(t *testing.T) { registeredSettingsContract(t, "WS") }
