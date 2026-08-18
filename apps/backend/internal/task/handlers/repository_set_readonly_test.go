package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// The HTTP mutations resolve a writable workspace before writing. The WebSocket
// mutations called the service directly, so a WebSocket client could create,
// update, or delete sets in the read-only Improve Kandev workspace.

// newImproveWorkspaceRouter seeds the read-only Improve workspace with one
// repository and one existing set, so every mutation has a real target.
func newImproveWorkspaceRouter(t *testing.T) (*gin.Engine, *ws.Dispatcher, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "repository-sets-readonly.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, cleanup())
		require.NoError(t, sqlxDB.Close())
	})

	ctx := context.Background()
	improve := &models.Workspace{ID: "ws-improve", Name: models.WorkspaceNameImproveKandev}
	require.NoError(t, repo.CreateWorkspace(ctx, improve))
	require.True(t, improve.IsImproveKandev(), "fixture must be the read-only workspace")
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-improve", WorkspaceID: "ws-improve", Name: "improve",
		SourceType: "local", LocalPath: t.TempDir(),
	}))
	existing := &models.RepositorySet{
		WorkspaceID: "ws-improve",
		Name:        "Existing",
		Items:       []models.RepositorySetItem{{RepositoryID: "repo-improve"}},
	}
	require.NoError(t, repo.CreateRepositorySet(ctx, existing))

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
	return router, dispatcher, existing.ID
}

func dispatchSetAction(t *testing.T, dispatcher *ws.Dispatcher, action string, payload any) *ws.Message {
	t.Helper()
	msg, err := ws.NewRequest("1", action, payload)
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	return response
}

func TestWSRepositorySetMutationsRejectReadOnlyWorkspace(t *testing.T) {
	router, dispatcher, setID := newImproveWorkspaceRouter(t)

	cases := []struct {
		name    string
		action  string
		payload any
	}{
		{"create", ws.ActionRepositorySetCreate, map[string]any{
			"workspace_id": "ws-improve", "name": "Blocked", "repository_ids": []string{"repo-improve"},
		}},
		{"update", ws.ActionRepositorySetUpdate, map[string]any{
			"id": setID, "name": "Renamed",
		}},
		{"delete", ws.ActionRepositorySetDelete, map[string]any{"id": setID}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := dispatchSetAction(t, dispatcher, testCase.action, testCase.payload)
			require.Equal(t, ws.MessageTypeError, response.Type,
				"%s must be rejected in the read-only workspace: %s", testCase.name, response.Payload)
		})
	}

	// The seeded set is untouched, and no new one was created.
	recorder := doJSON(t, router, http.MethodGet, "/api/v1/workspaces/ws-improve/repository-sets", nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	var list struct {
		RepositorySets []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"repository_sets"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &list))
	require.Len(t, list.RepositorySets, 1)
	require.Equal(t, setID, list.RepositorySets[0].ID)
	require.Equal(t, "Existing", list.RepositorySets[0].Name)
}

func TestWSRepositorySetReadsAreAllowedInReadOnlyWorkspace(t *testing.T) {
	_, dispatcher, setID := newImproveWorkspaceRouter(t)

	// Only mutations are blocked; the workspace stays readable.
	list := dispatchSetAction(t, dispatcher, ws.ActionRepositorySetList,
		map[string]any{"workspace_id": "ws-improve"})
	require.NotEqual(t, ws.MessageTypeError, list.Type, "list failed: %s", list.Payload)

	get := dispatchSetAction(t, dispatcher, ws.ActionRepositorySetGet, map[string]any{"id": setID})
	require.NotEqual(t, ws.MessageTypeError, get.Type, "get failed: %s", get.Payload)
}
