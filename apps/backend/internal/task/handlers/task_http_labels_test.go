package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

type labelTaskRepository struct {
	mockRepository
	mu         sync.Mutex
	tasks      map[string]*models.Task
	createCall int
}

func newLabelTaskRepository() *labelTaskRepository {
	return &labelTaskRepository{tasks: make(map[string]*models.Task)}
}

func (r *labelTaskRepository) GetWorkflow(_ context.Context, id string) (*models.Workflow, error) {
	return &models.Workflow{ID: id, WorkspaceID: "ws-1"}, nil
}

func (r *labelTaskRepository) CreateTask(_ context.Context, task *models.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *task
	r.tasks[task.ID] = &copy
	r.createCall++
	return nil
}

func (r *labelTaskRepository) GetTask(_ context.Context, id string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, taskrepo.ErrTaskNotFound
	}
	copy := *task
	return &copy, nil
}

func newLabelTaskRouter(t *testing.T, repo *labelTaskRepository) *gin.Engine {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	router := gin.New()
	h.registerHTTP(router)
	return router
}

func serveTaskCreate(router http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestHTTPCreateTaskLabelsPersistsAndRoundTrips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newLabelTaskRepository()
	router := newLabelTaskRouter(t, repo)

	created := serveTaskCreate(router, `{
		"workspace_id":"ws-1",
		"workflow_id":"wf-1",
		"title":"Label task",
		"labels":[" bug ","triage","bug","  "]
	}`)

	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	var createdBody struct {
		ID     string `json:"id"`
		Labels string `json:"labels"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdBody))
	assert.Equal(t, `["bug","triage"]`, createdBody.Labels)
	assert.Equal(t, 1, repo.createCall)

	stored, err := repo.GetTask(context.Background(), createdBody.ID)
	require.NoError(t, err)
	assert.Equal(t, `["bug","triage"]`, stored.Labels)

	read := httptest.NewRecorder()
	readRequest := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+createdBody.ID, nil)
	router.ServeHTTP(read, readRequest)

	require.Equal(t, http.StatusOK, read.Code, read.Body.String())
	var readBody struct {
		Labels string `json:"labels"`
	}
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &readBody))
	assert.Equal(t, `["bug","triage"]`, readBody.Labels)
}

func TestHTTPCreateTaskLabelsMissingAndEmptyUseDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		labelsField string
	}{
		{name: "missing"},
		{name: "empty", labelsField: `,"labels":[]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newLabelTaskRepository()
			router := newLabelTaskRouter(t, repo)
			body := `{"workspace_id":"ws-1","workflow_id":"wf-1","title":"Default labels"` + tc.labelsField + `}`

			created := serveTaskCreate(router, body)

			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			var response struct {
				ID     string `json:"id"`
				Labels string `json:"labels"`
			}
			require.NoError(t, json.Unmarshal(created.Body.Bytes(), &response))
			assert.Equal(t, `[]`, response.Labels)
			assert.Equal(t, 1, repo.createCall)

			stored, err := repo.GetTask(context.Background(), response.ID)
			require.NoError(t, err)
			assert.Equal(t, `[]`, stored.Labels)
		})
	}
}

func TestHTTPCreateTaskLabelsRejectsInvalidShapes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		labelsField string
	}{
		{name: "scalar", labelsField: `,"labels":"bug"`},
		{name: "mixed array", labelsField: `,"labels":["bug",7]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newLabelTaskRepository()
			router := newLabelTaskRouter(t, repo)
			body := `{"workspace_id":"ws-1","workflow_id":"wf-1","title":"Invalid labels"` + tc.labelsField + `}`

			created := serveTaskCreate(router, body)

			assert.Equal(t, http.StatusBadRequest, created.Code, created.Body.String())
			assert.Equal(t, 0, repo.createCall)
			assert.Empty(t, repo.tasks)
		})
	}
}
