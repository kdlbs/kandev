package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// captureUpdateTaskRepo backs the update-task tests: it serves a task with an
// attached repository and records whether the task's repository rows were
// wiped (the replace path always deletes before recreating).
type captureUpdateTaskRepo struct {
	mockRepository
	deleteReposCalled bool
}

func (m *captureUpdateTaskRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	return &models.Task{
		ID:          id,
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Old title",
		State:       v1.TaskStateTODO,
	}, nil
}

func (m *captureUpdateTaskRepo) DeleteTaskRepositoriesByTask(_ context.Context, _ string) error {
	m.deleteReposCalled = true
	return nil
}

func (m *captureUpdateTaskRepo) ListTaskRepositories(_ context.Context, taskID string) ([]*models.TaskRepository, error) {
	if m.deleteReposCalled {
		return nil, nil
	}
	return []*models.TaskRepository{
		{ID: "tr-1", TaskID: taskID, RepositoryID: "repo-1", BaseBranch: "main"},
	}, nil
}

func newUpdateTaskHandlers(t *testing.T) (*TaskHandlers, *captureUpdateTaskRepo) {
	t.Helper()
	log := newTestLogger(t)
	repo := &captureUpdateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}, repo
}

func decodeTaskRepositoryIDs(t *testing.T, data []byte) []string {
	t.Helper()
	var resp struct {
		Repositories []struct {
			RepositoryID string `json:"repository_id"`
		} `json:"repositories"`
	}
	require.NoError(t, json.Unmarshal(data, &resp))
	ids := make([]string, 0, len(resp.Repositories))
	for _, r := range resp.Repositories {
		ids = append(ids, r.RepositoryID)
	}
	return ids
}

func doUpdateTaskRequest(t *testing.T, h *TaskHandlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: "task-1"}}
	c.Request = httptest.NewRequest(http.MethodPatch, "/tasks/task-1", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.httpUpdateTask(c)
	return rec
}

// Regression: a title-only PATCH (rename) used to wipe every task_repositories
// row because the absent repositories field was converted to a non-nil empty
// slice, which Service.UpdateTask treats as an explicit replace-with-nothing.
func TestHTTPUpdateTaskTitleOnlyPreservesRepositories(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo := newUpdateTaskHandlers(t)

	rec := doUpdateTaskRequest(t, h, `{"title": "Renamed"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.False(t, repo.deleteReposCalled, "title-only update must not touch task repositories")
	assert.Equal(t, []string{"repo-1"}, decodeTaskRepositoryIDs(t, rec.Body.Bytes()))
}

func TestHTTPUpdateTaskExplicitEmptyRepositoriesClears(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo := newUpdateTaskHandlers(t)

	rec := doUpdateTaskRequest(t, h, `{"repositories": []}`)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.True(t, repo.deleteReposCalled, "an explicitly empty repositories list must still clear")
	assert.Empty(t, decodeTaskRepositoryIDs(t, rec.Body.Bytes()), "response must reflect the cleared repositories")
}

func TestWSUpdateTaskTitleOnlyPreservesRepositories(t *testing.T) {
	h, repo := newUpdateTaskHandlers(t)

	msg, err := ws.NewRequest("msg-1", ws.ActionTaskUpdate, map[string]any{
		"id":    "task-1",
		"title": "Renamed",
	})
	require.NoError(t, err)

	resp, err := h.wsUpdateTask(context.Background(), msg)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.False(t, repo.deleteReposCalled, "title-only update must not touch task repositories")
	assert.Equal(t, []string{"repo-1"}, decodeTaskRepositoryIDs(t, resp.Payload))
}

func TestConvertUpdateRepositories(t *testing.T) {
	assert.Nil(t, convertUpdateRepositories(false, nil), "absent field must stay nil")

	empty := convertUpdateRepositories(true, nil)
	require.NotNil(t, empty, "provided empty list must map to a non-nil slice so it clears")
	assert.Len(t, empty, 0)

	converted := convertUpdateRepositories(true, []dto.TaskRepositoryInput{{RepositoryID: "repo-1"}})
	require.Len(t, converted, 1)
	assert.Equal(t, "repo-1", converted[0].RepositoryID)
}
