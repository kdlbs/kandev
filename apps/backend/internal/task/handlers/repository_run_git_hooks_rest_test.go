package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type runGitHooksFixture struct {
	repo   *sqliterepo.Repository
	router *gin.Engine
	events *checkoutRouteEvents
}

func newRunGitHooksFixture(t *testing.T) *runGitHooksFixture {
	t.Helper()
	db, _ := workflowHandlerTemplate.Open(t)
	store := sqliterepo.NewWithInitializedDB(db, db, nil)
	log := newTestLogger(t)
	memory := bus.NewMemoryEventBus(log)
	t.Cleanup(memory.Close)
	events := &checkoutRouteEvents{EventBus: memory}
	api := service.NewService(service.Repos{Workspaces: store, RepoEntities: store}, events, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	RegisterRepositoryRoutes(router, ws.NewDispatcher(), api, log)
	require.NoError(t, store.CreateWorkspace(t.Context(), &models.Workspace{ID: "hooks-ws", Name: "Workspace", OwnerID: "hooks-owner"}))
	require.NoError(t, store.CreateRepository(t.Context(), &models.Repository{
		ID: "hooks-repo", WorkspaceID: "hooks-ws", Name: "before", DefaultBranch: "main", RunGitHooks: true,
	}))
	return &runGitHooksFixture{repo: store, router: router, events: events}
}

func (f *runGitHooksFixture) patch(t *testing.T, payload string) dto.RepositoryDTO {
	t.Helper()
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "hooks-owner"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/repositories/hooks-repo", strings.NewReader(payload)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var row dto.RepositoryDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &row))
	return row
}

func (f *runGitHooksFixture) stored(t *testing.T) bool {
	t.Helper()
	row, err := f.repo.GetRepository(t.Context(), "hooks-repo")
	require.NoError(t, err)
	return row.RunGitHooks
}

func TestRESTUpdateRepositoryRunGitHooksIsIntentGated(t *testing.T) {
	f := newRunGitHooksFixture(t)

	row := f.patch(t, `{"name":"renamed"}`)
	require.True(t, row.RunGitHooks, "an omitted field leaves the stored value")
	require.True(t, f.stored(t))

	row = f.patch(t, `{"run_git_hooks":null}`)
	require.True(t, row.RunGitHooks, "an explicit null leaves the stored value")
	require.True(t, f.stored(t))

	row = f.patch(t, `{"run_git_hooks":false}`)
	require.False(t, row.RunGitHooks)
	require.False(t, f.stored(t))

	row = f.patch(t, `{"name":"again"}`)
	require.False(t, row.RunGitHooks, "an unrelated update does not re-enable hooks")

	row = f.patch(t, `{"run_git_hooks":true}`)
	require.True(t, row.RunGitHooks)
	require.True(t, f.stored(t))
}

func TestRESTUpdateRepositoryEventCarriesStoredRunGitHooks(t *testing.T) {
	f := newRunGitHooksFixture(t)
	f.patch(t, `{"name":"renamed"}`)

	records := f.events.records()
	require.NotEmpty(t, records)
	last := records[len(records)-1]
	require.Equal(t, "repository.updated", last.Type)
	values, ok := last.Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, values["run_git_hooks"], "the event reports the stored value, not the request")
}

func TestRESTCreateRepositoryWithoutRunGitHooksIsOff(t *testing.T) {
	f := newRunGitHooksFixture(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "hooks-owner"})
	path := t.TempDir()
	initCmd := exec.Command("git", "init", "-q", "-b", "main", path)
	require.NoError(t, initCmd.Run())
	body := bytes.NewBufferString(`{"name":"fresh","source_type":"local","local_path":"` + path + `","default_branch":"main"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/hooks-ws/repositories", body).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	var row dto.RepositoryDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &row))
	require.False(t, row.RunGitHooks)
	stored, err := f.repo.GetRepository(t.Context(), row.ID)
	require.NoError(t, err)
	require.False(t, stored.RunGitHooks)
}
