package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/logger"
)

func newModelConfigRouter(t *testing.T) (*gin.Engine, *registry.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	reg := registry.NewRegistry(log)
	ctrl := controller.NewController(nil, nil, reg, nil, log)
	ctrl.SetHostUtility(hostutility.NewManager(reg, "127.0.0.1", 0, nil, log))
	router := gin.New()
	useSyntheticSettingsIdentity(router)
	RegisterRoutes(router, ctrl, nil, log, "test-interlock")
	return router, reg
}

func TestResolveAgentModelConfigEndpointRejectsInvalidJSON(t *testing.T) {
	router, _ := newModelConfigRouter(t)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-models/test-agent/resolve", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestResolveAgentModelConfigEndpointReturnsNotFoundForMissingAgent(t *testing.T) {
	router, _ := newModelConfigRouter(t)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agent-models/missing-agent/resolve",
		strings.NewReader(`{"model":"model"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestResolveAgentModelConfigEndpointDoesNotClassifyNonInferenceAgentAsNotFound(t *testing.T) {
	router, reg := newModelConfigRouter(t)
	requireNoError(t, reg.Register(agents.NewTUIAgent(agents.TUIAgentConfig{
		AgentID:   "terminal-agent",
		AgentName: "Terminal",
		Command:   "terminal-agent",
	})))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agent-models/terminal-agent/resolve",
		strings.NewReader(`{"model":"model"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
