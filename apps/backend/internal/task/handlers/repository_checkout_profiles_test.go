package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPCreateTaskExecutorProfileWithoutAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)

	repo := &captureCreateTaskRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{
		"workspace_id": "ws-1",
		"title": "Analyse integrations",
		"project_id": "proj-1",
		"priority": "medium", "executor_profile_id": "executor-profile"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.NotNil(t, repo.captured, "service.CreateTask was not called")
	assert.Equal(t, "executor-profile", repo.captured.Metadata[models.MetaKeyExecutorProfileID])
	assert.Equal(t, "wf-office", repo.captured.WorkflowID, "office workflow should be auto-resolved")
}
