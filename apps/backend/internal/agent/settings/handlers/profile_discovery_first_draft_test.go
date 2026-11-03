package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
)

const completeDraftSnapshot = `{"launch_settings":{"env_vars":[],"cli_flags":[],"command_prefix":"sandbox --"}}`

func postFirstDraftProbe(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/agent-models/devin-acp/probe",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

// newSettingsHarnessWithoutIdentity builds the production route table without
// an identity middleware so unauthenticated requests reach the scope guard.
func newSettingsHarnessWithoutIdentity(t *testing.T, repo store.Repository) (*gin.Engine, *registry.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	reg := registry.NewRegistry(log)
	ctrl := controller.NewController(repo, nil, reg, nil, log)
	router := gin.New()
	RegisterRoutes(router, ctrl, nil, log, "test-interlock")
	return router, reg
}

// @covers AC-AGENTS-PROFILE-DISCOVERY-001.2
// @covers AC-AGENTS-PROFILE-DISCOVERY-002.4
func TestFirstDraftProfileDiscoveryEndpoints(t *testing.T) {
	t.Run("probe accepts a complete draft without a parent row", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		router, _, reg := newSettingsHarness(t, repo, nil)
		if err := reg.Register(agents.NewDevinACP()); err != nil {
			t.Fatalf("register Devin: %v", err)
		}
		response := postFirstDraftProbe(t, router, completeDraftSnapshot)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s, want a successful draft probe", response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), `"agent_name":"devin-acp"`) {
			t.Fatalf("body = %s, want the Devin capability response", response.Body.String())
		}
		if len(repo.agents) != 0 {
			t.Fatalf("draft probe persisted agent rows: %+v", repo.agents)
		}
	})

	t.Run("resolve accepts a complete draft without a parent row", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		router, _, reg := newSettingsHarness(t, repo, nil)
		if err := reg.Register(agents.NewDevinACP()); err != nil {
			t.Fatalf("register Devin: %v", err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/agent-models/devin-acp/resolve",
			strings.NewReader(`{"model":"model-a","launch_settings":{"env_vars":[],"cli_flags":[],"command_prefix":""}}`),
		)
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s, want a successful draft resolution", response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), `"model":"model-a"`) {
			t.Fatalf("body = %s, want the resolved model", response.Body.String())
		}
		if len(repo.agents) != 0 {
			t.Fatalf("draft resolution persisted agent rows: %+v", repo.agents)
		}
	})

	t.Run("saved profile still requires its parent row", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		repo.putProfile(&models.AgentProfile{ID: "orphan-profile", AgentID: "deleted-parent"})
		router, _, reg := newSettingsHarness(t, repo, nil)
		if err := reg.Register(agents.NewDevinACP()); err != nil {
			t.Fatalf("register Devin: %v", err)
		}
		response := postFirstDraftProbe(t, router, `{"profile_id":"orphan-profile"}`)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s, want a missing-parent refusal", response.Code, response.Body.String())
		}
	})

	t.Run("unknown agent remains not found", func(t *testing.T) {
		router, _, _ := newSettingsHarness(t, newFakeSettingsRepo(), nil)
		response := postFirstDraftProbe(t, router, completeDraftSnapshot)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s, want unknown-agent refusal", response.Code, response.Body.String())
		}
	})

	t.Run("incomplete launch snapshot remains a bad request", func(t *testing.T) {
		router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
		if err := reg.Register(agents.NewDevinACP()); err != nil {
			t.Fatalf("register Devin: %v", err)
		}
		response := postFirstDraftProbe(t, router,
			`{"launch_settings":{"env_vars":[],"command_prefix":""}}`)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid profile discovery context") {
			t.Fatalf("status = %d, body = %s, want an incomplete-snapshot refusal", response.Code, response.Body.String())
		}
	})

	t.Run("missing identity remains unauthorized", func(t *testing.T) {
		router, reg := newSettingsHarnessWithoutIdentity(t, newFakeSettingsRepo())
		if err := reg.Register(agents.NewDevinACP()); err != nil {
			t.Fatalf("register Devin: %v", err)
		}
		response := postFirstDraftProbe(t, router, completeDraftSnapshot)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s, want an identity refusal", response.Code, response.Body.String())
		}
	})

	t.Run("missing config authority remains forbidden", func(t *testing.T) {
		router, _, reg := newSettingsHarnessAs(t, newFakeSettingsRepo(), nil,
			authn.Identity{UserID: "member-1", Role: authn.RoleMember})
		if err := reg.Register(agents.NewDevinACP()); err != nil {
			t.Fatalf("register Devin: %v", err)
		}
		response := postFirstDraftProbe(t, router, completeDraftSnapshot)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "org.config.manage") {
			t.Fatalf("status = %d, body = %s, want an org.config.manage refusal", response.Code, response.Body.String())
		}
	})
}

// @covers AC-AGENTS-DEVIN-PERMISSIONS-001.3
func TestSavedProfileDiscoveryPermissionIntentEndpoints(t *testing.T) {
	conflicting := `{"profile_id":"devin-profile","launch_settings":{"mode":"smart","env_vars":[{"key":"DEVIN_SANDBOX","value":"true"}],"cli_flags":[],"command_prefix":""}}`
	for _, route := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/agent-models/devin-acp/probe", body: conflicting},
		{
			path: "/api/v1/agent-models/devin-acp/resolve",
			body: `{"model":"model-a","mode":"smart","profile_id":"devin-profile","launch_settings":{"mode":"smart","env_vars":[{"key":"DEVIN_SANDBOX","value":"true"}],"cli_flags":[],"command_prefix":""}}`,
		},
	} {
		t.Run(route.path, func(t *testing.T) {
			repo := newFakeSettingsRepo()
			repo.putAgent(&models.Agent{ID: "devin-row", Name: "devin-acp"})
			repo.putProfile(&models.AgentProfile{
				ID: "devin-profile", AgentID: "devin-row", Mode: "smart",
				EnvVars: []models.ProfileEnvVar{{Key: "DEVIN_SANDBOX", Value: "false"}},
			})
			router, _, reg := newSettingsHarness(t, repo, nil)
			if err := reg.Register(agents.NewDevinACP()); err != nil {
				t.Fatalf("register Devin: %v", err)
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(route.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest ||
				!strings.Contains(response.Body.String(), "invalid Devin permission configuration") {
				t.Fatalf("status = %d, body = %s, want a permission-intent refusal", response.Code, response.Body.String())
			}
		})
	}
}
