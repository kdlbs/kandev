package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestRegisterRoutesRejectsMutationWithoutInterimSettingsInterlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	router := gin.New()
	NewHandlers(nil, nil, log, "test-interlock").registerHTTP(router)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestRegisterRoutesProtectsEveryStateChangingAgentSettingsRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	router := gin.New()
	NewHandlers(nil, nil, log, "test-interlock").registerHTTP(router)
	requests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/agents"},
		{method: http.MethodPost, path: "/api/v1/agents/tui"},
		{method: http.MethodPatch, path: "/api/v1/agents/agent-1"},
		{method: http.MethodDelete, path: "/api/v1/agents/agent-1"},
		{method: http.MethodPost, path: "/api/v1/agents/agent-1/profiles"},
		{method: http.MethodPost, path: "/api/v1/agent-install/agent-1"},
		{method: http.MethodPatch, path: "/api/v1/agent-profiles/profile-1"},
		{method: http.MethodDelete, path: "/api/v1/agent-profiles/profile-1"},
		{method: http.MethodPost, path: "/api/v1/agent-profiles/profile-1/mcp-config"},
	}

	for _, requestSpec := range requests {
		t.Run(requestSpec.method+" "+requestSpec.path, func(t *testing.T) {
			request := httptest.NewRequest(requestSpec.method, requestSpec.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
		})
	}
}
