package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
)

func TestAgentctlMetricsRouteReturnsSessionDiagnostics(t *testing.T) {
	var requestedMetrics string
	agentctl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedMetrics = r.URL.Query().Get("metrics")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"agentctl","label":"Execution","kind":"execution","metrics":[` +
			`{"id":"agentctl_goroutines","label":"Goroutines","available":true,"value":12},` +
			`{"id":"agentctl_git_poll_ms","label":"Git poll latency","unit":"ms","available":true,"value":3.5},` +
			`{"id":"agentctl_monitor_poll_ms","label":"Workspace monitor latency","unit":"ms","available":true,"value":4.5},` +
			`{"id":"agentctl_create_ready_ms","label":"Create-to-ready","unit":"ms","available":true,"value":42}` +
			`]}`))
	}))
	t.Cleanup(agentctl.Close)

	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, agentctl.URL, "sess-b"))
	router := gin.New()
	RegisterProcessRoutes(router, h.service, h.lifecycleMgr, h.logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/sess-b/agentctl/metrics", nil)
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{
		UserID: "user-b",
		Role:   authn.RoleMember,
	}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t,
		"agentctl_goroutines,agentctl_git_poll_ms,agentctl_monitor_poll_ms,agentctl_create_ready_ms",
		requestedMetrics,
	)
	var body struct {
		ID      string `json:"id"`
		Metrics []struct {
			ID        string   `json:"id"`
			Available bool     `json:"available"`
			Value     *float64 `json:"value"`
		} `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "agentctl", body.ID)
	require.Len(t, body.Metrics, 4)
	require.Equal(t, "agentctl_create_ready_ms", body.Metrics[3].ID)
	require.True(t, body.Metrics[3].Available)
	require.NotNil(t, body.Metrics[3].Value)
}

func TestAgentctlMetricsRouteDeniesForeignSession(t *testing.T) {
	called := false
	agentctl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(agentctl.Close)

	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, agentctl.URL, "sess-b"))
	router := gin.New()
	RegisterProcessRoutes(router, h.service, h.lifecycleMgr, h.logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/sess-b/agentctl/metrics", nil)
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{
		UserID: "user-a",
		Role:   authn.RoleMember,
	}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.False(t, called, "foreign requests must be denied before contacting agentctl")
}

func TestAgentctlMetricsRouteReportsUnavailableExecution(t *testing.T) {
	h := newForeignProcessHandlers(t, newLifecycleManager(t, newTestLogger(t)))
	router := gin.New()
	RegisterProcessRoutes(router, h.service, h.lifecycleMgr, h.logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/sess-b/agentctl/metrics", nil)
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{
		UserID: "user-b",
		Role:   authn.RoleMember,
	}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.JSONEq(t, `{"error":"agentctl not ready"}`, strings.TrimSpace(rec.Body.String()))
}
