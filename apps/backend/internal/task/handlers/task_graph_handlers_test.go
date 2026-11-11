package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type graphHandlerRepo struct {
	dependencyHandlerRepo
}

func (r *graphHandlerRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	if task := r.tasks[id]; task != nil {
		return task, nil
	}
	return nil, fmt.Errorf("%w: %s", taskrepo.ErrTaskNotFound, id)
}

func (r *graphHandlerRepo) ListChildren(_ context.Context, parentID string) ([]*models.Task, error) {
	var children []*models.Task
	for _, id := range []string{"child-a", "child-b", "parent", "solo"} {
		if task := r.tasks[id]; task != nil && task.ParentID == parentID && parentID != "" {
			children = append(children, task)
		}
	}
	return children, nil
}

func newGraphHandlerRepo() *graphHandlerRepo {
	childA := dependencyTask("child-a", "Child A")
	childA.ParentID = "parent"
	childA.State = v1.TaskStateCompleted
	childB := dependencyTask("child-b", "Child B")
	childB.ParentID = "parent"
	repo := &graphHandlerRepo{dependencyHandlerRepo{tasks: map[string]*models.Task{
		"parent":  dependencyTask("parent", "Parent"),
		"child-a": childA,
		"child-b": childB,
		"solo":    dependencyTask("solo", "Solo"),
	}}}
	repo.blockers = []*orchmodels.TaskBlocker{{TaskID: "child-b", BlockerTaskID: "child-a"}}
	return repo
}

func graphRequest(t *testing.T, repo *graphHandlerRepo, taskID, mode string) *httptest.ResponseRecorder {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetBlockerRepository(repo)
	h := &TaskHandlers{service: svc, logger: log}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	url := "/api/v1/tasks/" + taskID + "/graph"
	if mode != "" {
		url += "?mode=" + mode
	}
	c.Request = httptest.NewRequest(http.MethodGet, url, nil)
	c.Params = gin.Params{{Key: "id", Value: taskID}}
	h.httpGetTaskGraph(c)
	return recorder
}

func TestHTTPGetTaskGraphDefaultsToSubtasks(t *testing.T) {
	recorder := graphRequest(t, newGraphHandlerRepo(), "parent", "")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body dto.TaskGraphDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "subtasks", body.Mode)
	require.Equal(t, []string{"subtasks", "chain"}, body.AvailableModes)
	require.Equal(t, "parent", body.FocusTaskID)
	require.NotNil(t, body.Root)
	require.Equal(t, "Parent", body.Root.Title)
	require.Len(t, body.Nodes, 2)
	require.Equal(t, "child-a", body.Nodes[0].ID)
	require.Equal(t, "Child A", body.Nodes[0].Title)
	require.Equal(t, v1.TaskStateCompleted, body.Nodes[0].State)
	require.Equal(t, 1, body.Nodes[0].Depth)
	require.Equal(t, "member", body.Nodes[0].Role)
	require.Equal(t, "Parent", body.Nodes[0].ParentTitle)
	require.Equal(t, []dto.TaskGraphEdgeDTO{{From: "child-a", To: "child-b", Kind: "dependency", Status: "resolved"}}, body.Edges)
	require.False(t, body.Truncated)
}

func TestHTTPGetTaskGraphChainMode(t *testing.T) {
	recorder := graphRequest(t, newGraphHandlerRepo(), "child-b", "chain")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body dto.TaskGraphDTO
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "chain", body.Mode)
	require.Nil(t, body.Root)
	require.Len(t, body.Nodes, 2)
	require.Equal(t, "child-b", body.Nodes[0].ID)
	require.False(t, body.Nodes[0].Blocked)
	require.Len(t, body.Edges, 1)
}

func TestHTTPGetTaskGraphRejectsUnavailableMode(t *testing.T) {
	recorder := graphRequest(t, newGraphHandlerRepo(), "solo", "subtasks")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "graph_mode_unavailable", body["code"])
}

func TestHTTPGetTaskGraphMissingTaskIsNotFound(t *testing.T) {
	recorder := graphRequest(t, newGraphHandlerRepo(), "missing", "")

	require.Equal(t, http.StatusNotFound, recorder.Code)
}
