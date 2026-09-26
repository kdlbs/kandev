package configsync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
)

// newTestRouter mounts the config sync routes over a fresh service, exactly
// as internal/office/routes.go mounts them under the Office API group.
func newTestRouter(t *testing.T) (*gin.Engine, *Service, *fakeGitHub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, fg := newTestService(t)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/office"), NewHandler(svc, svc.logger))
	return router, svc, fg
}

func do(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out))
}

func TestRegisterRoutes_RegistersEveryEndpoint(t *testing.T) {
	router, _, _ := newTestRouter(t)
	want := map[string]bool{
		"GET /api/v1/office/workspaces/:wsId/config-sync/config":    true,
		"POST /api/v1/office/workspaces/:wsId/config-sync/config":   true,
		"DELETE /api/v1/office/workspaces/:wsId/config-sync/config": true,
		"POST /api/v1/office/workspaces/:wsId/config-sync/sync":     true,
	}
	for _, r := range router.Routes() {
		delete(want, r.Method+" "+r.Path)
	}
	for missing := range want {
		t.Errorf("route not registered: %s", missing)
	}
}

func TestHandler_GetConfig_NoneConfiguredReturns204(t *testing.T) {
	router, _, _ := newTestRouter(t)
	rec := do(t, router, http.MethodGet, "/api/v1/office/workspaces/ws-1/config-sync/config", "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_SetConfig_ValidPayloadReturns200(t *testing.T) {
	router, _, _ := newTestRouter(t)
	rec := do(t, router, http.MethodPost, "/api/v1/office/workspaces/ws-1/config-sync/config",
		`{"provider":"github","repo_owner":"acme","repo_name":"kandev-config","path":"cfg"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var cfg Config
	decodeBody(t, rec, &cfg)
	assert.Equal(t, "acme", cfg.RepoOwner)
	assert.Equal(t, "cfg", cfg.Path)
}

func TestHandler_SetConfig_InvalidPayloadReturns400(t *testing.T) {
	router, _, _ := newTestRouter(t)
	rec := do(t, router, http.MethodPost, "/api/v1/office/workspaces/ws-1/config-sync/config",
		`{"provider":"github"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_SetConfig_MalformedJSONReturns400(t *testing.T) {
	router, _, _ := newTestRouter(t)
	rec := do(t, router, http.MethodPost, "/api/v1/office/workspaces/ws-1/config-sync/config", `{`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DeleteConfig_ReturnsDeletedTrue(t *testing.T) {
	router, svc, _ := newTestRouter(t)
	_, err := svc.SetConfigForWorkspace(t.Context(), "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	rec := do(t, router, http.MethodDelete, "/api/v1/office/workspaces/ws-1/config-sync/config", "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	cfg, err := svc.GetConfigForWorkspace(t.Context(), "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestHandler_ForceSync_NotConfiguredReturns404(t *testing.T) {
	router, _, _ := newTestRouter(t)
	rec := do(t, router, http.MethodPost, "/api/v1/office/workspaces/ws-1/config-sync/sync", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_ForceSync_SuccessReturnsConfigAndResult(t *testing.T) {
	router, svc, fg := newTestRouter(t)
	_, err := svc.SetConfigForWorkspace(t.Context(), "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)
	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/ceo.yml")}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")

	rec := do(t, router, http.MethodPost, "/api/v1/office/workspaces/ws-1/config-sync/sync", "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body struct {
		Config Config     `json:"config"`
		Result SyncResult `json:"result"`
		Error  string     `json:"error"`
	}
	decodeBody(t, rec, &body)
	assert.Empty(t, body.Error)
	assert.True(t, body.Config.LastOk)
	assert.Equal(t, []string{"ceo"}, body.Result.Created)
}

// TestHandler_ForceSync_FailureReturns200WithErrorEmbedded pins the design's
// stated envelope shape: a forced run whose sync failed still returns 200,
// with the config updated and the error embedded, and no "result" key at all
// (there is deliberately no Error field on SyncResult itself).
func TestHandler_ForceSync_FailureReturns200WithErrorEmbedded(t *testing.T) {
	router, svc, fg := newTestRouter(t)
	_, err := svc.SetConfigForWorkspace(t.Context(), "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)
	// No "cfg" dir registered on fg: the root listing 404s and the walk fails.
	_ = fg

	rec := do(t, router, http.MethodPost, "/api/v1/office/workspaces/ws-1/config-sync/sync", "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body map[string]json.RawMessage
	decodeBody(t, rec, &body)
	assert.Contains(t, body, "error")
	assert.Contains(t, body, "config")
	assert.NotContains(t, body, "result")

	cfg, err := svc.GetConfigForWorkspace(t.Context(), "ws-1")
	require.NoError(t, err)
	assert.False(t, cfg.LastOk)
	assert.NotEmpty(t, cfg.LastError)
}
