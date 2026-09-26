package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// newRepositorySetTestRouter builds a real gin router over an in-memory task
// repository seeded with one workspace and three repositories.
func newRepositorySetTestRouter(t *testing.T) (*gin.Engine, *ws.Dispatcher) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "repository-sets.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, cleanup())
		require.NoError(t, sqlxDB.Close())
	})

	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}))
	for _, id := range []string{"repo-web", "repo-gateway", "repo-orders"} {
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: id, SourceType: "local", LocalPath: t.TempDir(),
		}))
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces:     repo,
		RepoEntities:   repo,
		RepositorySets: repo,
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})

	router := gin.New()
	dispatcher := ws.NewDispatcher()
	RegisterRepositorySetRoutes(router, dispatcher, svc, log)
	return router, dispatcher
}

func doJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func createSetViaHTTP(t *testing.T, router *gin.Engine, body any) dto.RepositorySetDTO {
	t.Helper()
	recorder := doJSON(t, router, http.MethodPost, "/api/v1/workspaces/ws-1/repository-sets", body)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	var created dto.RepositorySetDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &created))
	return created
}

func TestRegisterRepositorySetRoutesWiresHTTPAndWS(t *testing.T) {
	router, dispatcher := newRepositorySetTestRouter(t)

	requireRoutes(t, router,
		"GET /api/v1/workspaces/:id/repository-sets",
		"POST /api/v1/workspaces/:id/repository-sets",
		"GET /api/v1/repository-sets/:id",
		"PATCH /api/v1/repository-sets/:id",
		"DELETE /api/v1/repository-sets/:id",
	)
	requireActions(t, dispatcher,
		ws.ActionRepositorySetList,
		ws.ActionRepositorySetCreate,
		ws.ActionRepositorySetGet,
		ws.ActionRepositorySetUpdate,
		ws.ActionRepositorySetDelete,
	)
}

func TestHTTPCreateAndListRepositorySets(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)

	created := createSetViaHTTP(t, router, map[string]any{
		"name":           "Full-stack",
		"description":    "web + gateway",
		"repository_ids": []string{"repo-web", "repo-gateway"},
	})
	require.NotEmpty(t, created.ID)
	require.Equal(t, "ws-1", created.WorkspaceID)
	require.Len(t, created.Repositories, 2)
	require.Equal(t, "repo-web", created.Repositories[0].RepositoryID)
	require.Equal(t, 0, created.Repositories[0].Position)
	require.Equal(t, "repo-gateway", created.Repositories[1].RepositoryID)
	require.Equal(t, 1, created.Repositories[1].Position)

	recorder := doJSON(t, router, http.MethodGet, "/api/v1/workspaces/ws-1/repository-sets", nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	var list dto.ListRepositorySetsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &list))
	require.Equal(t, 1, list.Total)
	require.Len(t, list.RepositorySets, 1)
	require.Equal(t, created.ID, list.RepositorySets[0].ID)
}

func TestHTTPListRepositorySetsEmptyWorkspaceReturnsArray(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)

	recorder := doJSON(t, router, http.MethodGet, "/api/v1/workspaces/ws-1/repository-sets", nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	// The client indexes the list without a nil check.
	require.Contains(t, recorder.Body.String(), `"repository_sets":[]`)
}

func TestHTTPGetRepositorySet(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)
	created := createSetViaHTTP(t, router, map[string]any{
		"name":           "Full-stack",
		"repository_ids": []string{"repo-web"},
	})

	recorder := doJSON(t, router, http.MethodGet, "/api/v1/repository-sets/"+created.ID, nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	var got dto.RepositorySetDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	require.Equal(t, created.ID, got.ID)

	missing := doJSON(t, router, http.MethodGet, "/api/v1/repository-sets/nope", nil)
	require.Equal(t, http.StatusNotFound, missing.Code)
}

func TestHTTPCreateRepositorySetErrorCodes(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)
	createSetViaHTTP(t, router, map[string]any{
		"name":           "Full-stack",
		"repository_ids": []string{"repo-web"},
	})

	cases := []struct {
		name string
		body any
		want int
	}{
		{"blank name", map[string]any{"name": "  ", "repository_ids": []string{"repo-web"}}, http.StatusBadRequest},
		{"no members", map[string]any{"name": "Empty"}, http.StatusBadRequest},
		{"repeated member", map[string]any{
			"name": "Twice", "repository_ids": []string{"repo-web", "repo-web"},
		}, http.StatusBadRequest},
		{"duplicate name", map[string]any{
			"name": "full-STACK", "repository_ids": []string{"repo-orders"},
		}, http.StatusConflict},
		{"unknown member", map[string]any{
			"name": "Ghost", "repository_ids": []string{"repo-missing"},
		}, http.StatusUnprocessableEntity},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := doJSON(t, router, http.MethodPost,
				"/api/v1/workspaces/ws-1/repository-sets", testCase.body)
			require.Equal(t, testCase.want, recorder.Code, recorder.Body.String())
		})
	}
}

func TestHTTPCreateRepositorySetUnknownWorkspaceIsNotFound(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)

	recorder := doJSON(t, router, http.MethodPost, "/api/v1/workspaces/ws-missing/repository-sets",
		map[string]any{"name": "Ghost", "repository_ids": []string{"repo-web"}})
	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
}

func TestHTTPUpdateRepositorySetReplacesMembership(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)
	created := createSetViaHTTP(t, router, map[string]any{
		"name":           "Full-stack",
		"repository_ids": []string{"repo-web", "repo-gateway"},
	})

	recorder := doJSON(t, router, http.MethodPatch, "/api/v1/repository-sets/"+created.ID,
		map[string]any{"repository_ids": []string{"repo-orders", "repo-web"}})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var updated dto.RepositorySetDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &updated))
	require.Len(t, updated.Repositories, 2)
	require.Equal(t, "repo-orders", updated.Repositories[0].RepositoryID)
	require.Equal(t, "repo-web", updated.Repositories[1].RepositoryID)
	require.Equal(t, "Full-stack", updated.Name)
}

func TestHTTPUpdateRepositorySetOmittedMembershipIsPreserved(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)
	created := createSetViaHTTP(t, router, map[string]any{
		"name":           "Full-stack",
		"repository_ids": []string{"repo-web", "repo-gateway"},
	})

	// A rename must not be read as "remove every member".
	recorder := doJSON(t, router, http.MethodPatch, "/api/v1/repository-sets/"+created.ID,
		map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var updated dto.RepositorySetDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &updated))
	require.Equal(t, "Renamed", updated.Name)
	require.Len(t, updated.Repositories, 2)
}

func TestHTTPDeleteRepositorySet(t *testing.T) {
	router, _ := newRepositorySetTestRouter(t)
	created := createSetViaHTTP(t, router, map[string]any{
		"name":           "Full-stack",
		"repository_ids": []string{"repo-web"},
	})

	recorder := doJSON(t, router, http.MethodDelete, "/api/v1/repository-sets/"+created.ID, nil)
	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())

	again := doJSON(t, router, http.MethodDelete, "/api/v1/repository-sets/"+created.ID, nil)
	require.Equal(t, http.StatusNotFound, again.Code)
}

func TestWSRepositorySetListAndCreate(t *testing.T) {
	router, dispatcher := newRepositorySetTestRouter(t)
	ctx := context.Background()

	createMsg, err := ws.NewRequest("1", ws.ActionRepositorySetCreate, map[string]any{
		"workspace_id":   "ws-1",
		"name":           "Full-stack",
		"repository_ids": []string{"repo-web", "repo-gateway"},
	})
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(ctx, createMsg)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotEqual(t, ws.MessageTypeError, response.Type, "create failed: %s", response.Payload)

	listMsg, err := ws.NewRequest("2", ws.ActionRepositorySetList, map[string]any{"workspace_id": "ws-1"})
	require.NoError(t, err)
	listResponse, err := dispatcher.Dispatch(ctx, listMsg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, listResponse.Type, "list failed: %s", listResponse.Payload)

	var list dto.ListRepositorySetsResponse
	require.NoError(t, json.Unmarshal(listResponse.Payload, &list))
	require.Equal(t, 1, list.Total)

	// The HTTP surface sees the same row: one service, one store.
	httpList := doJSON(t, router, http.MethodGet, "/api/v1/workspaces/ws-1/repository-sets", nil)
	require.Equal(t, http.StatusOK, httpList.Code)
	require.Contains(t, httpList.Body.String(), "Full-stack")
}
