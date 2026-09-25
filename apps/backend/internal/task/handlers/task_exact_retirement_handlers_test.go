package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-EXACT-RETIREMENT-001.2
// @covers AC-TASKS-EXACT-RETIREMENT-001.4
func TestHTTPPreviewExactRetirementReturnsNoStoreUnknownReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC().Round(0)
	oldTask := &models.Task{ID: "old", WorkspaceID: "workspace", Title: "old", UpdatedAt: now}
	replacement := &models.Task{ID: "replacement", WorkspaceID: "workspace", Title: "replacement", UpdatedAt: now}
	repo := &moveTaskConflictRepo{tasks: map[string]*models.Task{"old": oldTask, "replacement": replacement}}
	h := newMovePreviewHandler(t, repo, nil, newTestLogger(t))
	router := gin.New()
	router.POST("/api/v1/tasks/:id/exact-retirement/preview", h.httpPreviewExactRetirement)
	body := `{"replacement_task_id":"replacement","workspace_id":"workspace","expected_old_generation":"` + now.Format(time.RFC3339Nano) + `","expected_replacement_generation":"` + now.Format(time.RFC3339Nano) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/old/exact-retirement/preview", strings.NewReader(body))
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "test", Synthetic: true}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Contains(t, rec.Body.String(), `"eligible":false`)
	require.Contains(t, rec.Body.String(), `"reason_code":"INVENTORY_UNAVAILABLE"`)
	require.Equal(t, now, oldTask.UpdatedAt)
}

// @covers AC-TASKS-EXACT-RETIREMENT-001.1
func TestHTTPPreviewExactRetirementRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &TaskHandlers{logger: newTestLogger(t)}
	router := gin.New()
	router.POST("/api/v1/tasks/:id/exact-retirement/preview", h.httpPreviewExactRetirement)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/old/exact-retirement/preview", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, `{"error":"authentication required"}`, rec.Body.String())
}
