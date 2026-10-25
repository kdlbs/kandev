package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestAgentMCPDiscoveryRouteReturnsUnsupportedForUnregisteredAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newMCPDiscoveryTestLogger(t)
	repo := newFakeSettingsRepo()
	repo.putAgent(&models.Agent{ID: "agent-row-1", Name: "mock-agent", SupportsMCP: true})
	reg := registry.NewRegistry(log)
	agent := agents.NewMockAgentWithID("mock-agent", "mock-agent", "Mock Agent")
	if err := reg.Register(agent); err != nil {
		t.Fatal(err)
	}
	h := NewHandlers(controller.NewController(repo, nil, reg, nil, log), nil, log, "")
	router := gin.New()
	h.registerHTTP(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-row-1/mcp-discovery", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got dto.AgentMCPDiscoveryDTO
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.AgentID != "agent-row-1" || got.Status != "unsupported" || got.ProviderID != "" || len(got.Servers) != 0 {
		t.Fatalf("unsupported response = %#v", got)
	}
}

func TestAgentMCPDiscoveryRouteReturnsNotFoundForUnknownAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newMCPDiscoveryTestLogger(t)
	reg := registry.NewRegistry(log)
	h := NewHandlers(controller.NewController(newFakeSettingsRepo(), nil, reg, nil, log), nil, log, "")
	router := gin.New()
	h.registerHTTP(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agents/missing/mcp-discovery", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestAgentMCPDiscoveryRouteDoesNotExposeRepositoryErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newMCPDiscoveryTestLogger(t)
	repo := newFakeSettingsRepo().failWith("GetAgent", errors.New("private storage detail"))
	reg := registry.NewRegistry(log)
	h := NewHandlers(controller.NewController(repo, nil, reg, nil, log), nil, log, "")
	router := gin.New()
	h.registerHTTP(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/mcp-discovery", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private storage detail") {
		t.Fatalf("response leaked repository error: %s", response.Body.String())
	}
}

func newMCPDiscoveryTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	return log
}
