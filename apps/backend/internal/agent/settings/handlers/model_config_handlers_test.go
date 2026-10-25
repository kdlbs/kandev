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
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
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

func TestResolveAgentModelConfigWithProfileContextRequiresConfigScope(t *testing.T) {
	router, _, _ := newSettingsHarnessAs(t, newFakeSettingsRepo(), nil,
		authn.Identity{UserID: "member-1", Role: authn.RoleMember})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agent-models/codex-acp/resolve",
		strings.NewReader(`{"model":"model","profile_id":"profile-1"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "org.config.manage") {
		t.Fatalf("status = %d, body = %s, want org.config.manage refusal", response.Code, response.Body.String())
	}
}

func TestProfileProbeRequiresCompleteDraftSnapshot(t *testing.T) {
	repo := newFakeSettingsRepo()
	repo.putAgent(&models.Agent{ID: "agent-row", Name: "codex-acp"})
	router, _, reg := newSettingsHarness(t, repo, nil)
	if err := reg.Register(agents.NewCodexACP()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-models/codex-acp/probe", strings.NewReader(
		`{"launch_settings":{"env_vars":[],"command_prefix":""}}`,
	))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid profile discovery context") {
		t.Fatalf("status = %d, body = %s, want incomplete snapshot refusal", response.Code, response.Body.String())
	}
}

func TestProfileProbeRejectsUnknownLaunchCommandField(t *testing.T) {
	router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
	if err := reg.Register(agents.NewCodexACP()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-models/codex-acp/probe", strings.NewReader(
		`{"launch_settings":{"env_vars":[],"cli_flags":[],"command_prefix":"","command":"sh"}}`,
	))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want unknown launch field refusal", response.Code, response.Body.String())
	}
}

func TestProfileProbeRejectsProfileOwnedByDifferentAgent(t *testing.T) {
	repo := newFakeSettingsRepo()
	repo.putAgent(&models.Agent{ID: "agent-row", Name: "codex-acp"})
	repo.putProfile(&models.AgentProfile{ID: "profile-1", AgentID: "another-agent-row"})
	router, _, reg := newSettingsHarness(t, repo, nil)
	if err := reg.Register(agents.NewCodexACP()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-models/codex-acp/probe", strings.NewReader(
		`{"profile_id":"profile-1"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "profile discovery target not found") {
		t.Fatalf("status = %d, body = %s, want generic profile not found", response.Code, response.Body.String())
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
