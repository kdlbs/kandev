package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
)

// mutatingSettingsRoutes is every state-changing agent-settings route. Agents,
// agent profiles and the runtimes behind them are org configuration, so each
// one is gated on org.config.manage.
var mutatingSettingsRoutes = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/api/v1/agents"},
	{http.MethodPost, "/api/v1/agents/tui"},
	{http.MethodPatch, "/api/v1/agents/tui/agent-1/mcp"},
	{http.MethodPatch, "/api/v1/agents/agent-1"},
	{http.MethodDelete, "/api/v1/agents/agent-1"},
	{http.MethodPost, "/api/v1/agents/agent-1/profiles"},
	{http.MethodPost, "/api/v1/agent-install/agent-1"},
	{http.MethodPost, "/api/v1/agent-update/agent-1"},
	{http.MethodPatch, "/api/v1/agent-profiles/profile-1"},
	{http.MethodDelete, "/api/v1/agent-profiles/profile-1"},
	{http.MethodPost, "/api/v1/agent-profiles/profile-1/duplicate"},
	{http.MethodPost, "/api/v1/agent-profiles/profile-1/mcp-config"},
}

func settingsRouterAs(t *testing.T, identity authn.Identity) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, identity)
		c.Next()
	})
	NewHandlers(nil, nil, log, "test-interlock").registerHTTP(router)
	return router
}

// TestAgentSettingsMutationsRequireOrgConfigManage pins the authorization gate
// on every mutating route. The interlock guard that used to be the only thing
// in front of these handlers answers 403 as well, so asserting the status alone
// would pass even with the scope check removed — the scope name in the body is
// what distinguishes an authorization refusal from a concurrent-edit refusal.
func TestAgentSettingsMutationsRequireOrgConfigManage(t *testing.T) {
	router := settingsRouterAs(t, authn.Identity{UserID: "member-1", Role: authn.RoleMember})

	for _, route := range mutatingSettingsRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, nil)
			request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if !strings.Contains(response.Body.String(), "org.config.manage") {
				t.Fatalf("body = %s, want an org.config.manage refusal", response.Body.String())
			}
		})
	}
}

// TestAgentSettingsReadsDoNotRequireOrgConfigManage keeps the gate off the read
// surface: a member must still be able to see the agents a task can run.
func TestAgentSettingsReadsDoNotRequireOrgConfigManage(t *testing.T) {
	// A real controller, not the nil one the refusal test can use: these reads
	// reach the handler.
	router, _, _ := newSettingsHarnessAs(t, newFakeSettingsRepo(), nil,
		authn.Identity{UserID: "member-1", Role: authn.RoleMember})

	for _, path := range []string{"/api/v1/agents", "/api/v1/agents/available"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code == http.StatusForbidden || response.Code == http.StatusUnauthorized {
				t.Fatalf("status = %d, want the read to pass the scope gate", response.Code)
			}
		})
	}
}
