package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

func TestHTTPPreviewExactRetirementAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo := newPlanTestHandlersWithRepo(t)
	ctx := context.Background()
	const workspaceID = "exact-retirement-private"
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Private", OwnerID: "owner"}))
	for _, id := range []string{"retirement-old", "retirement-replacement"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: workspaceID, Title: "private task"}))
	}
	require.NoError(t, repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: workspaceID, UserID: "viewer", Role: "viewer",
	}))
	require.NoError(t, repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: workspaceID, UserID: "collaborator", Role: "collaborator",
	}))
	log := h.logger
	h.service = service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo,
		Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo,
		Environments: repo, TaskEnvironments: repo, Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	oldTask, err := h.service.GetTask(ctx, "retirement-old")
	require.NoError(t, err)
	replacementTask, err := h.service.GetTask(ctx, "retirement-replacement")
	require.NoError(t, err)
	body := fmt.Sprintf(`{"replacement_task_id":"retirement-replacement","workspace_id":%q,"expected_old_generation":%q,"expected_replacement_generation":%q}`,
		workspaceID, oldTask.UpdatedAt.UTC().Format(time.RFC3339Nano), replacementTask.UpdatedAt.UTC().Format(time.RFC3339Nano))
	router := gin.New()
	router.POST("/api/v1/tasks/:id/exact-retirement/preview", h.httpPreviewExactRetirement)

	t.Run("viewer is denied without receipt", func(t *testing.T) {
		viewerBody := strings.Replace(body, oldTask.UpdatedAt.UTC().Format(time.RFC3339Nano), "stale-generation", 1)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/retirement-old/exact-retirement/preview", strings.NewReader(viewerBody))
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "viewer", Role: authn.RoleMember}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), `"receipts"`)
	})

	for _, tc := range []struct {
		name              string
		replacementTaskID string
	}{
		{name: "existing replacement", replacementTaskID: "retirement-replacement"},
		{name: "missing replacement", replacementTaskID: "missing-replacement"},
	} {
		t.Run("visible non-writer is denied before "+tc.name+" lookup", func(t *testing.T) {
			requestBody := strings.Replace(body, "retirement-replacement", tc.replacementTaskID, 1)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/retirement-old/exact-retirement/preview", strings.NewReader(requestBody))
			req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "viewer", Role: authn.RoleMember}))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), `"receipts"`)
		})
	}

	t.Run("foreign workspace is indistinguishable from missing task", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/retirement-old/exact-retirement/preview", strings.NewReader(body))
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "outsider", Role: authn.RoleMember}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), "private task")
		require.NotContains(t, rec.Body.String(), `"receipts"`)
	})

	t.Run("foreign caller with equal task IDs is indistinguishable from missing task", func(t *testing.T) {
		equalIDsBody := fmt.Sprintf(`{"replacement_task_id":"retirement-old","workspace_id":%q,"expected_old_generation":%q,"expected_replacement_generation":%q}`,
			workspaceID, oldTask.UpdatedAt.UTC().Format(time.RFC3339Nano), oldTask.UpdatedAt.UTC().Format(time.RFC3339Nano))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/retirement-old/exact-retirement/preview", strings.NewReader(equalIDsBody))
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "outsider", Role: authn.RoleMember}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), "private task")
		require.NotContains(t, rec.Body.String(), `"receipts"`)
	})

	t.Run("task writer without admin role is denied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/retirement-old/exact-retirement/preview", strings.NewReader(body))
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "collaborator", Role: authn.RoleMember}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		require.NotContains(t, rec.Body.String(), `"receipts"`)
	})

	t.Run("admin task writer receives blocked preview", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/retirement-old/exact-retirement/preview", strings.NewReader(body))
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "collaborator", Role: authn.RoleAdmin}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), `"eligible":false`)
		require.Contains(t, rec.Body.String(), `"status":"UNKNOWN"`)
	})
}
