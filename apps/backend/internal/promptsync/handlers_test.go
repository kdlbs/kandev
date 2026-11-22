package promptsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func newTestRouter(t *testing.T, svc *Service, identity authn.Identity) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, identity)
		c.Next()
	})
	RegisterRoutes(router, svc, log)
	return router
}

func doRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

var admin = authn.Identity{UserID: "admin", Role: authn.RoleAdmin, Synthetic: true}

func TestHandlersConfigLifecycleAndSync(t *testing.T) {
	f := newSyncFixture(t)
	router := newTestRouter(t, f.svc, admin)
	require.NoError(t, f.svc.DeleteConfig(context.Background(), Removal{}))

	assert.Equal(t, http.StatusNoContent, doRequest(router, http.MethodGet, "/api/v1/prompt-sync/config", "").Code)
	assert.Equal(t, http.StatusNotFound, doRequest(router, http.MethodPost, "/api/v1/prompt-sync/sync", "").Code)

	bad := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/config", `{"provider":"gitlab","project_path":"single","workspace_id":"ws-1"}`)
	assert.Equal(t, http.StatusBadRequest, bad.Code)
	assert.Contains(t, bad.Body.String(), "project_path")

	saved := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/config", `{"provider":"gitlab","project_path":"acme/eng","workspace_id":"ws-1"}`)
	require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
	assert.Contains(t, saved.Body.String(), `"path":".kandev/prompts"`)

	f.gitlab.files["a.md"] = "A"
	synced := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/sync", "")
	require.Equal(t, http.StatusOK, synced.Code)
	var body struct {
		Config *Config     `json:"config"`
		Result *SyncResult `json:"result"`
		Error  string      `json:"error"`
	}
	require.NoError(t, json.Unmarshal(synced.Body.Bytes(), &body))
	assert.Equal(t, []string{"a"}, body.Result.Created)
	assert.True(t, body.Config.LastOk)

	f.gitlab.listErr = &testAPIError{}
	failed := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/sync", "")
	require.Equal(t, http.StatusOK, failed.Code)
	assert.Contains(t, failed.Body.String(), `"error":"Prompt sync failed"`)

	deleted := doRequest(router, http.MethodDelete, "/api/v1/prompt-sync/config", "")
	assert.Equal(t, http.StatusOK, deleted.Code)
	assert.JSONEq(t, `{"deleted":true}`, deleted.Body.String())
}

func TestHandlerManualQuotaBlockIncludesRetryErrorCode(t *testing.T) {
	for _, route := range []struct {
		name string
		path string
		body string
	}{
		{name: "sync now", path: "/api/v1/prompt-sync/sync"},
		{
			name: "first sync after config save", path: "/api/v1/prompt-sync/config",
			body: `{"workspace_id":"ws-1","provider":"github","repo_owner":"acme","repo_name":"eng"}`,
		},
	} {
		t.Run(route.name, func(t *testing.T) {
			svc, gh, _ := newGitHubFixture(t)
			retryAt := time.Now().Add(time.Hour).UTC()
			gh.listErr = &github.AdmissionWaitError{
				Resource: github.ResourceCore, RetryAt: retryAt,
				RetrySource: github.RetrySourceRetryAfter, Reason: "primary_rate_limit",
				Cause: github.ErrGitHubAdmissionWaitRequired,
			}
			response := doRequest(newTestRouter(t, svc, admin), http.MethodPost, route.path, route.body)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var body struct {
				Config    Config `json:"config"`
				Error     string `json:"error"`
				ErrorCode string `json:"error_code"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, github.RateLimitErrorCode, body.ErrorCode)
			assert.Equal(t, quotaUnavailableMessage, body.Error)
			assert.Equal(t, quotaUnavailableMessage, body.Config.LastError)
			assert.Equal(t, github.RateLimitErrorCode, body.Config.LastErrorClass)
			require.NotNil(t, body.Config.NextAttemptAt)
			assert.Equal(t, retryAt, *body.Config.NextAttemptAt)
		})
	}
}

type testAPIError struct{}

func (*testAPIError) Error() string { return "upstream said: token=secret" }

func TestHandlersRequireConfigManageAndWorkspaceAccess(t *testing.T) {
	f := newSyncFixture(t)
	member := newTestRouter(t, f.svc, authn.Identity{UserID: "m", Role: authn.RoleMember, OrgID: "org-1"})
	for _, route := range [][2]string{
		{http.MethodPost, "/api/v1/prompt-sync/config"},
		{http.MethodDelete, "/api/v1/prompt-sync/config"},
		{http.MethodPost, "/api/v1/prompt-sync/sync"},
	} {
		response := doRequest(member, route[0], route[1], `{}`)
		assert.Equal(t, http.StatusForbidden, response.Code, route)
	}
	assert.Equal(t, http.StatusOK, doRequest(member, http.MethodGet, "/api/v1/prompt-sync/config", "").Code)

	f.svc.SetWorkspaceAuthorizer(func(_ context.Context, id string) error {
		if id == "foreign" {
			return repoerrors.ErrWorkspaceNotFound
		}
		return nil
	})
	router := newTestRouter(t, f.svc, admin)
	foreign := doRequest(router, http.MethodPost, "/api/v1/prompt-sync/config", `{"provider":"gitlab","project_path":"a/b","workspace_id":"foreign"}`)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
}
