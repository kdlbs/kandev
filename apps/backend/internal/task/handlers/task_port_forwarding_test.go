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
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

type portForwardingHandlerRepo struct {
	mockRepository
	task        *models.Task
	updatedTask *models.Task
	updateCalls int
}

func (r *portForwardingHandlerRepo) GetTask(_ context.Context, _ string) (*models.Task, error) {
	if r.task == nil {
		return nil, taskrepo.ErrTaskNotFound
	}
	return r.task, nil
}

func (r *portForwardingHandlerRepo) UpdateTask(_ context.Context, task *models.Task) error {
	r.updatedTask = task
	r.updateCalls++
	return nil
}

func newPortForwardingHandler(t *testing.T, repo *portForwardingHandlerRepo) *TaskHandlers {
	t.Helper()
	taskService := service.NewService(service.Repos{
		Tasks:      repo,
		Workflows:  repo,
		Workspaces: repo,
	}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: taskService, logger: newTestLogger(t)}
}

func TestHTTPUpdateTaskPortForwardingMergesMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &portForwardingHandlerRepo{task: &models.Task{
		ID:          "task-port-forwarding",
		WorkspaceID: "workspace-1",
		Metadata: map[string]interface{}{
			"unrelated":                         "preserve",
			models.MetaKeyPortForwardingEnabled: false,
		},
	}}
	handlers := newPortForwardingHandler(t, repo)
	router := gin.New()
	handlers.registerHTTP(router)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/task-port-forwarding/port-forwarding", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, 1, repo.updateCalls)
	require.NotNil(t, repo.updatedTask)
	assert.Equal(t, "preserve", repo.updatedTask.Metadata["unrelated"])
	assert.Equal(t, true, repo.updatedTask.Metadata[models.MetaKeyPortForwardingEnabled])

	var response dto.TaskDTO
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	assert.Equal(t, true, response.Metadata[models.MetaKeyPortForwardingEnabled])
}

func TestHTTPUpdateTaskPortForwardingRejectsInvalidBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing enabled", body: `{}`},
		{name: "null enabled", body: `{"enabled":null}`},
		{name: "non boolean enabled", body: `{"enabled":"true"}`},
		{name: "unknown field", body: `{"enabled":true,"unexpected":true}`},
		{name: "malformed json", body: `{"enabled":`},
		{name: "trailing json value", body: `{"enabled":true} {"enabled":false}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			repo := &portForwardingHandlerRepo{task: &models.Task{ID: "task-port-forwarding"}}
			handlers := newPortForwardingHandler(t, repo)
			router := gin.New()
			handlers.registerHTTP(router)

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/task-port-forwarding/port-forwarding", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)

			assert.Equal(t, http.StatusBadRequest, res.Code)
			assert.Zero(t, repo.updateCalls)
		})
	}
}

func TestHTTPUpdateTaskPortForwardingReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &portForwardingHandlerRepo{}
	handlers := newPortForwardingHandler(t, repo)
	router := gin.New()
	handlers.registerHTTP(router)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/missing-task/port-forwarding", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	assert.Equal(t, http.StatusNotFound, res.Code)
	assert.Zero(t, repo.updateCalls)
}
