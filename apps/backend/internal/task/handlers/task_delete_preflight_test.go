package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

type taskDeletePreflightHTTPCleanup struct {
	dirty bool
}

func (taskDeletePreflightHTTPCleanup) OnTaskDeleted(context.Context, string) error { return nil }

func (taskDeletePreflightHTTPCleanup) GetAllByTaskID(
	context.Context, string,
) ([]*worktree.Worktree, error) {
	return []*worktree.Worktree{{ID: "wt-1", TaskID: "task-1"}}, nil
}

func (c taskDeletePreflightHTTPCleanup) InspectDirtyWorktrees(
	context.Context, []*worktree.Worktree,
) ([]worktree.DirtyWorktree, error) {
	if !c.dirty {
		return nil, nil
	}
	return []worktree.DirtyWorktree{{WorktreeID: "wt-1"}}, nil
}

func newTaskDeletePreflightRouter(t *testing.T, cleanup service.WorktreeCleanup) *gin.Engine {
	t.Helper()
	log := newTestLogger(t)
	repo := &mockRepository{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorktreeCleanup(cleanup)
	h := &TaskHandlers{service: svc, logger: log}
	router := gin.New()
	h.registerHTTP(router)
	return router
}

func TestHTTPTaskDeletePreflightReturnsNoStoreConsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTaskDeletePreflightRouter(t, taskDeletePreflightHTTPCleanup{dirty: true})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/tasks/delete-preflight",
		strings.NewReader(`{"task_ids":["task-1"],"cascade":false}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var response service.TaskDeletePreflightResult
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.RequiresDiscardConsent)
}

func TestHTTPTaskDeletePreflightRejectsEmptySelection(t *testing.T) {
	router := newTaskDeletePreflightRouter(t, taskDeletePreflightHTTPCleanup{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/tasks/delete-preflight",
		strings.NewReader(`{"task_ids":[],"cascade":false}`),
	)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
}

func TestHTTPTaskDeletePreflightReturnsServiceUnavailableWhenInspectionUnavailable(t *testing.T) {
	router := newTaskDeletePreflightRouter(t, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/tasks/delete-preflight",
		strings.NewReader(`{"task_ids":["task-1"],"cascade":false}`),
	)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
}
