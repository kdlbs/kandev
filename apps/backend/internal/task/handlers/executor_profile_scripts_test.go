package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type profileTransportSnapshot struct {
	*tasksqlite.Repository
	afterRead func(context.Context) error
}

func (r *profileTransportSnapshot) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	profile, err := r.Repository.GetExecutorProfile(ctx, id)
	if err != nil || r.afterRead == nil {
		return profile, err
	}
	hook := r.afterRead
	r.afterRead = nil
	return profile, hook(ctx)
}

type profileTransportFixture struct {
	gate     *profileTransportSnapshot
	repo     *tasksqlite.Repository
	other    *service.Service
	router   *gin.Engine
	dispatch *ws.Dispatcher
	updates  chan *bus.Event
}

func newProfileTransportFixture(t *testing.T) profileTransportFixture {
	t.Helper()
	a, path := workflowHandlerTemplate.Open(t)
	raw, err := db.OpenSQLite(path)
	require.NoError(t, err)
	b := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	repo := tasksqlite.NewWithInitializedDB(a, a, nil)
	otherRepo := tasksqlite.NewWithInitializedDB(b, b, nil)
	require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{ID: "script-executor", Name: "Local", Type: models.ExecutorTypeLocal}))
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), &models.ExecutorProfile{
		ID: "script-profile", ExecutorID: "script-executor", Name: "Before", PrepareScript: "before-prepare", CleanupScript: "before-cleanup",
	}))
	log := newTestLogger(t)
	eventsBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventsBus.Close)
	f := profileTransportFixture{gate: &profileTransportSnapshot{Repository: repo}, repo: repo, router: gin.New(), dispatch: ws.NewDispatcher(), updates: make(chan *bus.Event, 4)}
	_, err = eventsBus.Subscribe(events.ExecutorProfileUpdated, func(_ context.Context, event *bus.Event) error { f.updates <- event; return nil })
	require.NoError(t, err)
	svc := service.NewService(service.Repos{Executors: f.gate}, eventsBus, log, service.RepositoryDiscoveryConfig{})
	f.other = service.NewService(service.Repos{Executors: otherRepo}, nil, log, service.RepositoryDiscoveryConfig{})
	RegisterExecutorProfileRoutes(f.router, f.dispatch, svc, nil, log)
	return f
}

func (f profileTransportFixture) save(t *testing.T, transport, id string, payload map[string]any, success bool) *models.ExecutorProfile {
	t.Helper()
	var data []byte
	if transport == "HTTP" {
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/executors/script-executor/profiles/"+id, strings.NewReader(string(body))).WithContext(t.Context())
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if !success {
			require.NotEqual(t, http.StatusOK, rec.Code)
			return nil
		}
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		data = rec.Body.Bytes()
	} else {
		payload["id"] = id
		req, err := ws.NewRequest("scripts", ws.ActionExecutorProfileUpdate, payload)
		require.NoError(t, err)
		response, err := f.dispatch.Dispatch(t.Context(), req)
		require.NoError(t, err)
		if !success {
			require.Equal(t, ws.MessageTypeError, response.Type)
			return nil
		}
		require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
		data = response.Payload
	}
	var profile models.ExecutorProfile
	require.NoError(t, json.Unmarshal(data, &profile))
	return &profile
}

func testRegisteredProfileScripts(t *testing.T, transport string) {
	f := newProfileTransportFixture(t)
	prepare, cleanup := "saved-prepare", "saved-cleanup"
	f.gate.afterRead = func(ctx context.Context) error {
		_, err := f.other.UpdateExecutorProfile(ctx, "script-profile", &service.UpdateExecutorProfileRequest{PrepareScript: &prepare, CleanupScript: &cleanup})
		return err
	}
	returned := f.save(t, transport, "script-profile", map[string]any{"name": "Renamed"}, true)
	require.Equal(t, prepare, returned.PrepareScript)
	require.Equal(t, cleanup, returned.CleanupScript)
	stored, err := f.repo.GetExecutorProfile(t.Context(), returned.ID)
	require.NoError(t, err)
	require.Equal(t, returned.PrepareScript, stored.PrepareScript)
	require.Equal(t, returned.CleanupScript, stored.CleanupScript)
	require.True(t, returned.UpdatedAt.Equal(stored.UpdatedAt))
	select {
	case event := <-f.updates:
		data := event.Data.(map[string]interface{})
		require.Equal(t, prepare, data["prepare_script"])
		require.Equal(t, cleanup, data["cleanup_script"])
		require.Equal(t, returned.UpdatedAt.Format(time.RFC3339), data["updated_at"])
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	cleared := f.save(t, transport, "script-profile", map[string]any{"prepare_script": "", "cleanup_script": ""}, true)
	require.Empty(t, cleared.PrepareScript)
	require.Empty(t, cleared.CleanupScript)
	<-f.updates
	f.save(t, transport, "missing-profile", map[string]any{"name": "Rejected"}, false)
	select {
	case event := <-f.updates:
		t.Fatalf("failed update published %#v", event)
	default:
	}
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.8 AC-EXECUTORS-PROFILE-EDITOR-001.9 AC-EXECUTORS-PROFILE-EDITOR-001.10 AC-EXECUTORS-PROFILE-EDITOR-001.11
func TestRegisteredExecutorProfileScriptsHTTP(t *testing.T) { testRegisteredProfileScripts(t, "HTTP") }

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.8 AC-EXECUTORS-PROFILE-EDITOR-001.9 AC-EXECUTORS-PROFILE-EDITOR-001.10 AC-EXECUTORS-PROFILE-EDITOR-001.11
func TestRegisteredExecutorProfileScriptsWS(t *testing.T) { testRegisteredProfileScripts(t, "WS") }
