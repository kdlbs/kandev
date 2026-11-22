package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type commandPanelHTTPRepo struct {
	*httpTaskRepo
	workspace, workflow string
}

func (r *commandPanelHTTPRepo) ListTasksByWorkspaceForCommandPanel(_ context.Context, workspace, workflow, _, _ string, _, _ int, _ string, _, _, _, _, _ bool) ([]*models.Task, int, error) {
	r.workspace, r.workflow = workspace, workflow
	return []*models.Task{{ID: "task-b", WorkspaceID: workspace, Title: "Mine"}}, 1, nil
}

func newCommandPanelHTTPHandlers(t *testing.T, repo *commandPanelHTTPRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo, ResourceCleanups: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}
}

// @covers AC-TASKS-WORKSPACE-SEARCH-002.3
func TestCommandPanelEligibleRejectsInvalidRequest(t *testing.T) {
	for _, query := range []string{
		"?command_panel_visible=true",
		"?command_panel_visible=true&workflow_id=flow&query=needle",
	} {
		t.Run(query, func(t *testing.T) {
			repo := &httpTaskRepo{}
			h := newHTTPTaskHandlers(t, repo)
			c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workspaces/ws-b/tasks"+query, "ws-b")
			h.httpListTasksByWorkspace(c)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Empty(t, repo.listedWorkspaceID)
		})
	}
}

func TestCommandPanelEligibleHTTPAuthorizesAndUsesCapability(t *testing.T) {
	repo := &commandPanelHTTPRepo{httpTaskRepo: &httpTaskRepo{}}
	h := newCommandPanelHTTPHandlers(t, repo)
	url := "/api/v1/workspaces/ws-b/tasks?command_panel_visible=true&workflow_id=flow&page=1&page_size=20"
	c, rec := taskRequestAs(t, "user-a", http.MethodGet, url, "ws-b")
	h.httpListTasksByWorkspace(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, repo.workflow)

	c, rec = taskRequestAs(t, "", http.MethodGet, url, "ws-b")
	h.httpListTasksByWorkspace(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ws-b", repo.workspace)
	require.Equal(t, "flow", repo.workflow)
	var result dto.ListTasksResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Equal(t, 1, result.Total)
	require.Len(t, result.Tasks, 1)
}

func TestCommandPanelEligibleUnavailableFailsClosed(t *testing.T) {
	repo := &httpTaskRepo{}
	h := newHTTPTaskHandlers(t, repo)
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workspaces/ws-b/tasks?command_panel_visible=true&workflow_id=flow", "ws-b")
	h.httpListTasksByWorkspace(c)
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.Empty(t, repo.listedWorkspaceID)
}
