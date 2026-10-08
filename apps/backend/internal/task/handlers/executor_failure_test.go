package handlers

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecutorFailureHTTPRecheckIsReadOnlyAndRevisionBound(t *testing.T) {
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "executor-failure.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	repo, err := sqliterepo.NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	ctx := t.Context()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf", WorkspaceID: "ws", Name: "Workflow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "ws", WorkflowID: "wf", WorkflowStepID: "step", Title: "Executor failure", Priority: "medium"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env", TaskID: "task", OwnershipGeneration: 1, ExecutorType: "local_docker", Status: models.TaskEnvironmentStatusReady, ContainerID: "owned"}))
	observed := time.Now().UTC()
	episode, _, err := repo.ObserveExecutorFailure(ctx, models.ExecutorObservationTarget{TaskID: "task", EnvironmentID: "env", OwnershipGeneration: 1, ResourceKey: "owned", Runtime: "docker"}, &models.ExecutorObservation{Outcome: "terminated", Runtime: "docker", ResourceKey: "owned", ObservedAt: observed, Reason: "ContainerExited", Workspace: "unknown"})
	require.NoError(t, err)
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{Tasks: repo, Sessions: repo, TaskEnvironments: repo, StatusSummaries: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	inspections := 0
	svc.SetExecutorInspector(func(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
		inspections++
		return &models.ExecutorObservation{Outcome: "healthy", ResourceKey: "owned", ObservedAt: observed.Add(time.Minute), Workspace: "unknown"}, nil
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewTaskHandlers(svc, nil, repo, nil, log).registerHTTP(router)
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task/executor-failure/recheck", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusBadRequest, post(`{}`).Code)
	payload, _ := json.Marshal(map[string]any{"episode_id": episode.ID, "revision": episode.Revision + 100})
	require.Equal(t, http.StatusConflict, post(string(payload)).Code)
	require.Zero(t, inspections)
	payload, _ = json.Marshal(map[string]any{"episode_id": episode.ID, "revision": episode.Revision})
	response := post(string(payload))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, 1, inspections)
	var result models.ExecutorFailureEpisode
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, "resolved", result.State)
	environment, err := repo.GetTaskEnvironment(ctx, "env")
	require.NoError(t, err)
	require.Equal(t, "owned", environment.ContainerID)
	require.EqualValues(t, 1, environment.OwnershipGeneration)
}

func TestExecutorFailureHTTPRecheckDeniesForeignOwnerBeforeBinding(t *testing.T) {
	h := newForeignSessionHandlers(t)
	c, response := requestAs(t, "user-a", "task-b")
	c.Request.Body = io.NopCloser(strings.NewReader("malformed body"))
	h.httpRecheckExecutorFailure(c)
	require.Equal(t, http.StatusNotFound, response.Code, "authorization precedes payload parsing or any resource inspection")
	require.NotContains(t, response.Body.String(), "executor")
}
