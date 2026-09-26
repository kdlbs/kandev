package workflowsync

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestRouter(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	router := gin.New()
	RegisterRoutes(router, svc, log)
	return router
}

func doJSON(t *testing.T, router *gin.Engine, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHandlers_SetConfig_GitLabRoundtrip(t *testing.T) {
	svc, _ := setupGitLabTestService(t, nil)
	router := newTestRouter(t, svc)

	w := doJSON(t, router, http.MethodPost, "/api/v1/workflow-sync/config?workspace_id=ws-1", map[string]any{
		"provider":     "gitlab",
		"project_path": "acme/team/project",
	})
	require.Equal(t, http.StatusOK, w.Code)

	var cfg Config
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cfg))
	assert.Equal(t, ProviderGitLab, cfg.Provider)
	assert.Equal(t, "acme/team/project", cfg.ProjectPath)
	assert.Empty(t, cfg.RepoOwner)

	w = doJSON(t, router, http.MethodGet, "/api/v1/workflow-sync/config?workspace_id=ws-1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var reread Config
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reread))
	assert.Equal(t, ProviderGitLab, reread.Provider)
	assert.Equal(t, "acme/team/project", reread.ProjectPath)
}

// Omitting provider entirely must still mean GitHub, so pre-GitLab API
// clients keep working unchanged.
func TestHandlers_SetConfig_OmittedProviderDefaultsToGitHub(t *testing.T) {
	svc, _ := setupTestService(t, nil)
	router := newTestRouter(t, svc)

	w := doJSON(t, router, http.MethodPost, "/api/v1/workflow-sync/config?workspace_id=ws-1", map[string]any{
		"repo_owner": "acme",
		"repo_name":  "flows",
	})
	require.Equal(t, http.StatusOK, w.Code)

	var cfg Config
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cfg))
	assert.Equal(t, ProviderGitHub, cfg.Provider)
}

func TestHandlers_SetConfig_InvalidProviderReturns400(t *testing.T) {
	svc, _ := setupTestService(t, nil)
	router := newTestRouter(t, svc)

	w := doJSON(t, router, http.MethodPost, "/api/v1/workflow-sync/config?workspace_id=ws-1", map[string]any{
		"provider": "bitbucket",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
