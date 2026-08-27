package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/httpmw"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// fixedSettingsTime pins every seeded row's timestamps so a response body can
// be compared whole rather than field by field.
var fixedSettingsTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// registerTestTUIAgent registers a deterministic agent implementation under the
// given ID. A TUI agent is the only built-in whose logo, billing type, display
// order and command are all fixed by its config, so assertions on them do not
// depend on what is installed on the machine running the test.
func registerTestTUIAgent(t *testing.T, reg *registry.Registry, id string, order int) {
	t.Helper()
	if err := reg.Register(agents.NewTUIAgent(agents.TUIAgentConfig{
		AgentID:   id,
		AgentName: id + "-name",
		Display:   id + " Display",
		Command:   id + "-binary",
		Order:     &order,
		LogoLight: []byte("<svg id=\"light\"/>"),
		LogoDark:  []byte("<svg id=\"dark\"/>"),
	})); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
}

// seedAgent adds an agent row with pinned timestamps.
func seedAgent(repo *fakeSettingsRepo, id, name string, supportsMCP bool) *models.Agent {
	return repo.putAgent(&models.Agent{
		ID:          id,
		Name:        name,
		SupportsMCP: supportsMCP,
		CreatedAt:   fixedSettingsTime,
		UpdatedAt:   fixedSettingsTime,
	})
}

// seedProfile adds a profile row with pinned timestamps.
func seedProfile(repo *fakeSettingsRepo, id, agentID, name, workspaceID string) *models.AgentProfile {
	return repo.putProfile(&models.AgentProfile{
		ID:               id,
		AgentID:          agentID,
		Name:             name,
		AgentDisplayName: "Display",
		Model:            "model-a",
		WorkspaceID:      workspaceID,
		Enabled:          true,
		CreatedAt:        fixedSettingsTime,
		UpdatedAt:        fixedSettingsTime,
	})
}

// wantProfileDTO is the DTO the seeded profile is expected to serialize to.
func wantProfileDTO(id, agentID, name, billingType string) dto.AgentProfileDTO {
	return dto.AgentProfileDTO{
		ID:               id,
		AgentID:          agentID,
		Kind:             "concrete",
		Name:             name,
		AgentDisplayName: "Display",
		Model:            "model-a",
		CLIFlags:         []dto.CLIFlagDTO{},
		Enabled:          true,
		BillingType:      billingType,
		CreatedAt:        fixedSettingsTime,
		UpdatedAt:        fixedSettingsTime,
	}
}

func doSettingsRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// decodeSettingsJSON decodes a response body, failing the test on bad JSON.
func decodeSettingsJSON(t *testing.T, response *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), into); err != nil {
		t.Fatalf("decode body %s: %v", response.Body.String(), err)
	}
}

// requireErrorBody asserts the response is exactly {"error": <want>}.
func requireErrorBody(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantError string) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, wantStatus, response.Body.String())
	}
	var body map[string]string
	decodeSettingsJSON(t, response, &body)
	if body["error"] != wantError {
		t.Fatalf("error = %q, want %q", body["error"], wantError)
	}
}

// TestListAgentsEndpointReturnsRegistryOrderAndHidesOfficeProfiles asserts the
// whole GET /agents body: agents are ranked by the registry's DisplayOrder
// rather than store order, workspace-scoped (office) profiles are withheld,
// and billing type is filled in from the registered agent implementation.
func TestListAgentsEndpointReturnsRegistryOrderAndHidesOfficeProfiles(t *testing.T) {
	repo := newFakeSettingsRepo()
	seedAgent(repo, "agent-late", "late-agent", false)
	seedAgent(repo, "agent-early", "early-agent", true)
	seedProfile(repo, "profile-1", "agent-early", "Kanban", "")
	seedProfile(repo, "profile-2", "agent-early", "Office", "ws-1")

	router, _, reg := newSettingsHarness(t, repo, nil)
	registerTestTUIAgent(t, reg, "early-agent", 1)
	registerTestTUIAgent(t, reg, "late-agent", 2)

	response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got dto.ListAgentsResponse
	decodeSettingsJSON(t, response, &got)

	want := dto.ListAgentsResponse{
		Total: 2,
		Agents: []dto.AgentDTO{
			{
				ID: "agent-early", Name: "early-agent", SupportsMCP: true,
				Profiles:  []dto.AgentProfileDTO{wantProfileDTO("profile-1", "agent-early", "Kanban", "api_key")},
				CreatedAt: fixedSettingsTime, UpdatedAt: fixedSettingsTime,
			},
			{
				ID: "agent-late", Name: "late-agent",
				Profiles:  []dto.AgentProfileDTO{},
				CreatedAt: fixedSettingsTime, UpdatedAt: fixedSettingsTime,
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response =\n%#v\nwant\n%#v", got, want)
	}
}

func TestListAgentsEndpointReportsStoreFailure(t *testing.T) {
	repo := newFakeSettingsRepo().failWith("ListAgents", errors.New("database is locked"))
	router := newSettingsRouter(t, repo, nil)

	response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents", "")
	requireErrorBody(t, response, http.StatusInternalServerError, "failed to list agents")
}

// TestGetAgentEndpoint distinguishes the missing-row 404 from a store failure,
// and pins the success body including the withheld office profile.
func TestGetAgentEndpoint(t *testing.T) {
	t.Run("returns the agent with only its kanban profiles", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "test-agent", true)
		seedProfile(repo, "profile-1", "agent-1", "Kanban", "")
		seedProfile(repo, "profile-2", "agent-1", "Office", "ws-1")
		router, _, reg := newSettingsHarness(t, repo, nil)
		registerTestTUIAgent(t, reg, "test-agent", 1)

		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/agent-1", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentDTO
		decodeSettingsJSON(t, response, &got)
		want := dto.AgentDTO{
			ID: "agent-1", Name: "test-agent", SupportsMCP: true,
			Profiles:  []dto.AgentProfileDTO{wantProfileDTO("profile-1", "agent-1", "Kanban", "api_key")},
			CreatedAt: fixedSettingsTime, UpdatedAt: fixedSettingsTime,
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("response =\n%#v\nwant\n%#v", got, want)
		}
	})

	t.Run("unknown id is 404", func(t *testing.T) {
		router := newSettingsRouter(t, newFakeSettingsRepo(), nil)
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/missing", "")
		requireErrorBody(t, response, http.StatusNotFound, "agent not found")
	})

	t.Run("store failure is 500, not 404", func(t *testing.T) {
		repo := newFakeSettingsRepo().failWith("GetAgent", errors.New("database is locked"))
		router := newSettingsRouter(t, repo, nil)
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/agent-1", "")
		requireErrorBody(t, response, http.StatusInternalServerError, "failed to get agent")
	})
}

// TestCreateAgentEndpointRejectsInvalidRequests covers every 400 branch of
// httpCreateAgent, each with the specific message the handler emits.
func TestCreateAgentEndpointRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantError string
	}{
		{name: "malformed json", body: "{", wantError: "invalid payload"},
		{name: "missing name", body: `{"profiles":[]}`, wantError: "name is required"},
		{
			name:      "blank nested profile name",
			body:      `{"name":"test-agent","profiles":[{"name":"  ","model":"m"}]}`,
			wantError: "profile name is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeSettingsRepo()
			router := newSettingsRouter(t, repo, nil)
			response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents", tc.body)
			requireErrorBody(t, response, http.StatusBadRequest, tc.wantError)
			if len(repo.agents) != 0 {
				t.Fatalf("rejected create still wrote %d agent rows", len(repo.agents))
			}
		})
	}
}

// TestUpdateAgentEndpoint asserts the persisted mutation as well as the body:
// a handler that returned the right JSON without writing the row would pass a
// body-only assertion.
func TestUpdateAgentEndpoint(t *testing.T) {
	t.Run("applies every optional field and persists it", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "test-agent", false)
		router := newSettingsRouter(t, repo, nil)

		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agents/agent-1",
			`{"workspace_id":"ws-9","supports_mcp":true,"mcp_config_path":"/tmp/mcp.json"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentDTO
		decodeSettingsJSON(t, response, &got)
		if got.WorkspaceID == nil || *got.WorkspaceID != "ws-9" {
			t.Errorf("workspace_id = %v, want ws-9", got.WorkspaceID)
		}
		if !got.SupportsMCP || got.MCPConfigPath != "/tmp/mcp.json" {
			t.Errorf("supports_mcp/mcp_config_path = %v/%q", got.SupportsMCP, got.MCPConfigPath)
		}
		if len(repo.updatedAgents) != 1 {
			t.Fatalf("UpdateAgent called %d times, want 1", len(repo.updatedAgents))
		}
		stored := repo.updatedAgents[0]
		if stored.ID != "agent-1" || !stored.SupportsMCP || stored.MCPConfigPath != "/tmp/mcp.json" {
			t.Fatalf("persisted agent = %#v", stored)
		}
	})

	t.Run("unknown id is 404", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		router := newSettingsRouter(t, repo, nil)
		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agents/missing", `{"supports_mcp":true}`)
		requireErrorBody(t, response, http.StatusNotFound, "agent not found")
		if len(repo.updatedAgents) != 0 {
			t.Fatalf("404 path still wrote %d agent rows", len(repo.updatedAgents))
		}
	})

	t.Run("malformed json is 400", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "test-agent", false)
		router := newSettingsRouter(t, repo, nil)
		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agents/agent-1", "{")
		requireErrorBody(t, response, http.StatusBadRequest, "invalid payload")
	})
}

// TestDeleteAgentEndpoint covers the success body, the deletion actually
// reaching the store, the async available-agents broadcast, and the 404.
func TestDeleteAgentEndpoint(t *testing.T) {
	t.Run("deletes the row and broadcasts the refreshed agent list", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "test-agent", false)
		hub := newSignallingHub()
		router := newSettingsRouter(t, repo, hub)

		response := doSettingsRequest(router, http.MethodDelete, "/api/v1/agents/agent-1", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var body map[string]bool
		decodeSettingsJSON(t, response, &body)
		if !body["success"] {
			t.Fatalf("body = %s, want success true", response.Body.String())
		}
		if !reflect.DeepEqual(repo.deletedAgents, []string{"agent-1"}) {
			t.Fatalf("deleted agents = %v, want [agent-1]", repo.deletedAgents)
		}
		hub.awaitAction(t, ws.ActionAgentAvailableUpdated)
	})

	t.Run("unknown id is 404 and broadcasts nothing", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		hub := &duplicateHub{}
		router := newSettingsRouter(t, repo, hub)

		response := doSettingsRequest(router, http.MethodDelete, "/api/v1/agents/missing", "")
		requireErrorBody(t, response, http.StatusNotFound, "agent not found")
		if len(hub.actions()) != 0 {
			t.Fatalf("404 delete broadcast %v", hub.actions())
		}
	})

	t.Run("store failure is 500", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "test-agent", false)
		repo.failWith("DeleteAgent", errors.New("database is locked"))
		router := newSettingsRouter(t, repo, nil)

		response := doSettingsRequest(router, http.MethodDelete, "/api/v1/agents/agent-1", "")
		requireErrorBody(t, response, http.StatusInternalServerError, "failed to delete agent")
	})
}

// TestGetAgentLogoEndpoint pins the SVG bytes, the variant switch and the
// caching header, and separates "no such agent" from "agent has no logo" —
// both 404s, but only one of them means the agent is unknown.
func TestGetAgentLogoEndpoint(t *testing.T) {
	t.Run("serves the light logo by default and the dark variant on request", func(t *testing.T) {
		router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
		registerTestTUIAgent(t, reg, "logo-agent", 1)

		light := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/logo-agent/logo", "")
		if light.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", light.Code, light.Body.String())
		}
		if got := light.Body.String(); got != `<svg id="light"/>` {
			t.Errorf("light body = %q", got)
		}
		if got := light.Header().Get("Content-Type"); got != "image/svg+xml" {
			t.Errorf("Content-Type = %q, want image/svg+xml", got)
		}
		if got := light.Header().Get("Cache-Control"); got != "public, max-age=86400" {
			t.Errorf("Cache-Control = %q", got)
		}

		dark := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/logo-agent/logo?variant=dark", "")
		if got := dark.Body.String(); got != `<svg id="dark"/>` {
			t.Errorf("dark body = %q", got)
		}
	})

	t.Run("unknown agent reports agent not found", func(t *testing.T) {
		router := newSettingsRouter(t, newFakeSettingsRepo(), nil)
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/nope/logo", "")
		requireErrorBody(t, response, http.StatusNotFound, "agent not found")
	})

	t.Run("registered agent without a logo reports logo not available", func(t *testing.T) {
		router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
		order := 1
		if err := reg.Register(agents.NewTUIAgent(agents.TUIAgentConfig{
			AgentID: "bare-agent", AgentName: "bare", Command: "bare", Order: &order,
		})); err != nil {
			t.Fatalf("register bare agent: %v", err)
		}
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/bare-agent/logo", "")
		requireErrorBody(t, response, http.StatusNotFound, "logo not available for agent")
	})
}

// TestPreviewAgentCommandEndpoint asserts the rendered argv, the launcher
// prefix and the CLI-flag tokens, plus both failure branches.
func TestPreviewAgentCommandEndpoint(t *testing.T) {
	t.Run("renders the argv including prefix and enabled cli flags", func(t *testing.T) {
		router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
		registerTestTUIAgent(t, reg, "preview-agent", 1)

		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-command-preview/preview-agent",
			`{"model":"m","command_prefix":"greywall --","cli_flags":[{"flag":"--verbose","enabled":true}]}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.CommandPreviewResponse
		decodeSettingsJSON(t, response, &got)
		want := dto.CommandPreviewResponse{
			Supported:     true,
			Command:       []string{"greywall", "--", "preview-agent-binary", "--verbose"},
			CommandString: "greywall -- preview-agent-binary --verbose",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("preview = %#v, want %#v", got, want)
		}
	})

	t.Run("malformed json is 400", func(t *testing.T) {
		router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
		registerTestTUIAgent(t, reg, "preview-agent", 1)
		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-command-preview/preview-agent", "{")
		requireErrorBody(t, response, http.StatusBadRequest, "invalid payload")
	})

	t.Run("unregistered agent is 500", func(t *testing.T) {
		router := newSettingsRouter(t, newFakeSettingsRepo(), nil)
		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-command-preview/nope", `{"model":"m"}`)
		requireErrorBody(t, response, http.StatusInternalServerError, `agent type "nope" not found in registry`)
	})
}

// TestInstallJobEndpoints covers the job list/lookup pair and each distinct
// enqueue rejection: an unknown agent is 404, a known agent with no install
// script is 400, and a backend without a WS hub reports the job store as
// unavailable rather than pretending the agent is missing.
func TestInstallJobEndpoints(t *testing.T) {
	router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), newSignallingHub())
	registerTestTUIAgent(t, reg, "scriptless-agent", 1)

	list := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-install/jobs", "")
	if list.Code != http.StatusOK || list.Body.String() != `{"jobs":[]}` {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}

	missing := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-install/jobs/nope", "")
	requireErrorBody(t, missing, http.StatusNotFound, "job not found")

	unknownAgent := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-install/ghost", "")
	requireErrorBody(t, unknownAgent, http.StatusNotFound, "agent not found")

	scriptless := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-install/scriptless-agent", "")
	requireErrorBody(t, scriptless, http.StatusBadRequest, "agent has no install script")

	hubless, _, hublessReg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
	registerTestTUIAgent(t, hublessReg, "scriptless-agent", 1)
	unavailable := doSettingsRequest(hubless, http.MethodPost, "/api/v1/agent-install/scriptless-agent", "")
	requireErrorBody(t, unavailable, http.StatusServiceUnavailable, "install service not ready")
}
