package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type workspaceDeleteRepo struct {
	mockRepository
	deleteCalled     bool
	cascadeCalled    bool
	cascadeID        string
	cascadeName      string
	cascadeTasks     []*models.Task
	cascadeWorkflows []*models.Workflow
	getCalls         int
	getErr           error
}

func (r *workspaceDeleteRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	r.getCalls++
	if r.getErr != nil {
		return nil, r.getErr
	}
	return &models.Workspace{ID: id, Name: "Delete Me"}, nil
}

func (r *workspaceDeleteRepo) DeleteWorkspace(_ context.Context, _ string) error {
	r.deleteCalled = true
	return nil
}

func (r *workspaceDeleteRepo) DeleteWorkspaceCascadeWithName(_ context.Context, id, name string) ([]*models.Task, []*models.Workflow, error) {
	r.cascadeCalled = true
	r.cascadeID = id
	r.cascadeName = name
	return r.cascadeTasks, r.cascadeWorkflows, nil
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
	require.False(t, repo.cascadeCalled, "handler must not cascade when confirm_name does not match")
	require.Equal(t, 1, repo.getCalls, "handler should leave confirmation lookup to the service")
}

func TestHTTPDeleteWorkspaceRejectsInvalidPayload(t *testing.T) {
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
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/ws-1", strings.NewReader(`{`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpDeleteWorkspace(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"invalid payload"}`, rec.Body.String())
	require.Equal(t, 0, repo.getCalls, "invalid payload must not look up the workspace")
	require.False(t, repo.deleteCalled, "invalid payload must not delete")
	require.False(t, repo.cascadeCalled, "invalid payload must not cascade")
}

func TestHTTPDeleteWorkspaceReturnsNotFoundWhenWorkspaceMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &workspaceDeleteRepo{getErr: errors.New("workspace not found")}
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
		"/api/v1/workspaces/ws-missing",
		strings.NewReader(`{"confirm_name":"Delete Me"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-missing"}}

	h.httpDeleteWorkspace(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"workspace not found"}`, rec.Body.String())
	require.Equal(t, 1, repo.getCalls, "handler should let service resolve the workspace")
	require.False(t, repo.deleteCalled, "missing workspace must not delete")
	require.False(t, repo.cascadeCalled, "missing workspace must not cascade")
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
	require.False(t, repo.deleteCalled, "handler must use the confirmed cascade path")
	require.True(t, repo.cascadeCalled, "handler must cascade when confirm_name matches")
	require.Equal(t, "ws-1", repo.cascadeID)
	require.Equal(t, "Delete Me", repo.cascadeName)
	require.Equal(t, 1, repo.getCalls, "confirmed delete should fetch the workspace once")
}

func TestWSDeleteWorkspaceRequiresConfirmName(t *testing.T) {
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
	req, err := ws.NewRequest("req-1", ws.ActionWorkspaceDelete, map[string]string{"id": "ws-1"})
	require.NoError(t, err)

	resp, err := h.wsDeleteWorkspace(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	require.Contains(t, string(resp.Payload), "confirm_name is required")
	require.Equal(t, 0, repo.getCalls, "missing confirm_name must not look up the workspace")
	require.False(t, repo.deleteCalled, "missing confirm_name must not delete")
	require.False(t, repo.cascadeCalled, "missing confirm_name must not cascade")
}

func TestWSDeleteWorkspaceDeletesWhenConfirmNameMatches(t *testing.T) {
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
	req, err := ws.NewRequest("req-1", ws.ActionWorkspaceDelete, map[string]string{
		"id":           "ws-1",
		"confirm_name": "Delete Me",
	})
	require.NoError(t, err)

	resp, err := h.wsDeleteWorkspace(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.False(t, repo.deleteCalled, "handler must use the confirmed cascade path")
	require.True(t, repo.cascadeCalled, "handler must cascade when confirm_name matches")
	require.Equal(t, "ws-1", repo.cascadeID)
	require.Equal(t, "Delete Me", repo.cascadeName)
	require.Equal(t, 1, repo.getCalls, "confirmed delete should fetch the workspace once")
}
