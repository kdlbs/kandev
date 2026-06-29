package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type workspaceDeleteRepo struct {
	mockRepository
	deleteCalled bool
	getCalls     int
}

func (r *workspaceDeleteRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	r.getCalls++
	return &models.Workspace{ID: id, Name: "Delete Me"}, nil
}

func (r *workspaceDeleteRepo) DeleteWorkspace(_ context.Context, _ string) error {
	r.deleteCalled = true
	return nil
}

func TestHTTPDeleteWorkspaceRequiresMatchingConfirmName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &workspaceDeleteRepo{}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := service.NewService(service.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})

	h := NewWorkspaceHandlers(svc, log)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/workspaces/ws-1",
		strings.NewReader(`{"confirm_name":"Wrong"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpDeleteWorkspace(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, repo.deleteCalled, "handler must not delete when confirm_name does not match")
	require.Equal(t, 1, repo.getCalls, "handler should leave confirmation lookup to the service")
}

func TestHTTPDeleteWorkspaceDeletesWhenConfirmNameMatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &workspaceDeleteRepo{}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := service.NewService(service.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})

	h := NewWorkspaceHandlers(svc, log)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/workspaces/ws-1",
		strings.NewReader(`{"confirm_name":"Delete Me"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpDeleteWorkspace(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.deleteCalled, "handler must delete when confirm_name matches")
	require.Equal(t, 1, repo.getCalls, "confirmed delete should fetch the workspace once")
}
