package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/api/v1"
)

func TestHTTPGetTaskSessionConditional(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newSessionHandlerService(t, &models.TaskSession{
		ID:     "sess-conditional",
		TaskID: "task-1",
		State:  models.TaskSessionStateRunning,
	})
	activity := &orchestratorWithActivity{value: v1.ForegroundActivityGenerating}
	h := &TaskHandlers{service: svc, foregroundActivity: activity, logger: newTestLogger(t)}

	first := getConditionalTaskSessionRecorder(t, h, "sess-conditional", "")
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), `"foreground_activity":"generating"`)
	etag := first.Header().Get("ETag")
	require.True(t, ifNoneMatch([]string{etag}, etag))
	require.NotEmpty(t, etag)
	require.Regexp(t, `^"[a-f0-9]{64}"$`, etag)
	require.Equal(t, "private, no-cache", first.Header().Get("Cache-Control"))

	different := getConditionalTaskSessionRecorder(t, h, "sess-conditional", `"another-session-version"`)
	require.Equal(t, http.StatusOK, different.Code)
	require.Contains(t, different.Body.String(), `"foreground_activity":"generating"`)
	require.Equal(t, etag, different.Header().Get("ETag"))

	unchanged := getConditionalTaskSessionRecorder(t, h, "sess-conditional", etag)
	require.Equal(t, http.StatusNotModified, unchanged.Code)
	require.Empty(t, unchanged.Body.Bytes())
	require.Equal(t, etag, unchanged.Header().Get("ETag"))

	activity.value = v1.ForegroundActivityBackground
	changed := getConditionalTaskSessionRecorder(t, h, "sess-conditional", etag)
	require.Equal(t, http.StatusOK, changed.Code)
	require.Contains(t, changed.Body.String(), `"foreground_activity":"background"`)
	require.NotEqual(t, etag, changed.Header().Get("ETag"))
}

func TestHTTPGetTaskSessionConditionalAuthorizesBeforeMatching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newForeignSessionHandlers(t)
	initial := getTaskSessionRecorderAs(t, h, "user-b", "sess-b", "")
	require.Equal(t, http.StatusOK, initial.Code, "owner must reach the session response")
	etag := initial.Header().Get("ETag")
	require.NotEmpty(t, etag)

	matchingOwner := getTaskSessionRecorderAs(t, h, "user-b", "sess-b", etag)
	require.Equal(t, http.StatusNotModified, matchingOwner.Code, "fixture must exercise a matching validator")
	require.Empty(t, matchingOwner.Body.Bytes())

	ctx, rec := newTaskSessionRequest(t, "user-a", "sess-b", etag)

	h.httpGetTaskSession(ctx)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotEqual(t, http.StatusNotModified, rec.Code)
	require.Empty(t, rec.Header().Get("ETag"))
	require.Contains(t, rec.Body.String(), `"task session not found"`)
}

func getTaskSessionRecorderAs(t *testing.T, h *TaskHandlers, userID, id, etag string) *httptest.ResponseRecorder {
	t.Helper()
	ctx, rec := newTaskSessionRequest(t, userID, id, etag)
	h.httpGetTaskSession(ctx)
	return rec
}

func newTaskSessionRequest(t *testing.T, userID, id, etag string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/"+id, nil)
	request = request.WithContext(authn.WithIdentity(
		request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember},
	))
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	ctx.Request = request
	ctx.Params = gin.Params{{Key: "id", Value: id}}
	return ctx, rec
}

func getConditionalTaskSessionRecorder(t *testing.T, h *TaskHandlers, id, etag string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/task-sessions/"+id, nil).WithContext(context.Background())
	if etag != "" {
		c.Request.Header.Set("If-None-Match", etag)
	}
	c.Params = gin.Params{{Key: "id", Value: id}}
	h.httpGetTaskSession(c)
	return rec
}
