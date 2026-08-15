package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestDiscoverAgentsEndpointInvalidatesCacheAndBroadcasts asserts the scan
// endpoint returns the discovery snapshot and then pushes the refreshed
// available-agents list, which is what flips the settings page out of its
// pre-scan state without a reload.
func TestDiscoverAgentsEndpointInvalidatesCacheAndBroadcasts(t *testing.T) {
	hub := newSignallingHub()
	router := newSettingsRouter(t, newFakeSettingsRepo(), hub)

	response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/discovery", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got dto.ListDiscoveryResponse
	decodeSettingsJSON(t, response, &got)
	if got.Total != 0 || len(got.Agents) != 0 {
		t.Fatalf("discovery = %#v, want an empty snapshot for an empty registry", got)
	}
	hub.awaitAction(t, ws.ActionAgentAvailableUpdated)
}

// TestListAvailableAgentsEndpointIncludesUninstalledAgents pins that a
// registered agent the host does not have installed is still listed (with
// available=false) rather than dropped, and that the endpoint broadcasts
// nothing of its own.
func TestListAvailableAgentsEndpointIncludesUninstalledAgents(t *testing.T) {
	hub := &duplicateHub{}
	router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), hub)
	registerTestTUIAgent(t, reg, "absent-agent", 1)

	response := doSettingsRequest(router, http.MethodGet, "/api/v1/agents/available", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got dto.ListAvailableAgentsResponse
	decodeSettingsJSON(t, response, &got)
	if got.Total != 1 || len(got.Agents) != 1 {
		t.Fatalf("available = %#v, want exactly the registered agent", got.Agents)
	}
	agent := got.Agents[0]
	if agent.Name != "absent-agent" || agent.DisplayName != "absent-agent Display" {
		t.Errorf("agent identity = %q / %q", agent.Name, agent.DisplayName)
	}
	if agent.Available {
		t.Errorf("agent %q reported available although discovery never found it", agent.Name)
	}
	if len(hub.actions()) != 0 {
		t.Fatalf("read-only list broadcast %v", hub.actions())
	}
}

// TestGetAgentModelsEndpoint covers the two reachable outcomes of
// /agent-models/:agentName.
func TestGetAgentModelsEndpoint(t *testing.T) {
	t.Run("registered agent without a probe reports not_configured", func(t *testing.T) {
		router, _, reg := newSettingsHarness(t, newFakeSettingsRepo(), nil)
		registerTestTUIAgent(t, reg, "model-agent", 1)

		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-models/model-agent", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.DynamicModelsResponse
		decodeSettingsJSON(t, response, &got)
		if got.AgentName != "model-agent" || got.Status != "not_configured" || got.Error != nil {
			t.Fatalf("models response = %#v", got)
		}
	})

	// FetchDynamicModels returns controller.ErrAgentNotFound for an agent the
	// registry does not know, which is the sentinel the handler's 404 branch
	// matches on. The body is the handler's fixed message: unlike the 500
	// branch it must not echo the controller error, so the caller-supplied
	// agent name never reaches the wire.
	t.Run("unknown agent is 404 with a fixed body", func(t *testing.T) {
		router := newSettingsRouter(t, newFakeSettingsRepo(), nil)
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-models/ghost", "")
		requireErrorBody(t, response, http.StatusNotFound, "agent not found")
		if strings.Contains(response.Body.String(), "ghost") {
			t.Errorf("404 body %s echoes the raw error", response.Body.String())
		}
	})
}

// TestCreateCustomTUIAgentEndpoint covers the full create: the persisted agent
// and its seeded passthrough profile, both broadcasts, the two field
// validations, and the duplicate-slug conflict.
func TestCreateCustomTUIAgentEndpoint(t *testing.T) {
	t.Run("registers the agent, seeds a passthrough profile, and broadcasts twice", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		hub := &duplicateHub{}
		router, _, reg := newSettingsHarness(t, repo, hub)

		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/tui",
			`{"display_name":"My CLI","command":"my-cli","command_args":["--flag","with space"],"model":"fast"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentDTO
		decodeSettingsJSON(t, response, &got)
		if got.Name != "my-cli" {
			t.Errorf("agent name = %q, want the slugified display name", got.Name)
		}
		if got.TUIConfig == nil || got.TUIConfig.Command != "my-cli" || !got.TUIConfig.WaitForTerminal {
			t.Fatalf("tui config = %#v", got.TUIConfig)
		}
		if len(got.TUIConfig.CommandArgs) != 2 || got.TUIConfig.CommandArgs[1] != "with space" {
			t.Errorf("command args = %#v, want the spaced element preserved", got.TUIConfig.CommandArgs)
		}
		if len(got.Profiles) != 1 || got.Profiles[0].Name != "fast" || !got.Profiles[0].CLIPassthrough {
			t.Fatalf("seeded profiles = %#v", got.Profiles)
		}
		if _, registered := reg.Get("my-cli"); !registered {
			t.Error("agent was not registered in the in-memory registry")
		}
		if len(repo.created) != 1 {
			t.Fatalf("stored profiles = %#v", repo.created)
		}
		actions := hub.actions()
		if len(actions) != 2 ||
			actions[0] != ws.ActionAgentAvailableUpdated ||
			actions[1] != ws.ActionAgentProfileCreated {
			t.Fatalf("broadcasts = %v, want the available list then the new profile", actions)
		}
	})

	t.Run("rejections", func(t *testing.T) {
		cases := []struct {
			name       string
			body       string
			wantStatus int
			wantError  string
		}{
			{name: "malformed json", body: "{", wantStatus: http.StatusBadRequest, wantError: "invalid payload"},
			{
				name: "missing display name", body: `{"command":"my-cli"}`,
				wantStatus: http.StatusBadRequest, wantError: "display_name is required",
			},
			{
				name: "missing command", body: `{"display_name":"My CLI"}`,
				wantStatus: http.StatusBadRequest, wantError: "command is required",
			},
			{
				// Punctuation-only names slugify to the empty string, which
				// would otherwise register an agent with no ID.
				name: "display name with no slug characters", body: `{"display_name":"!!!","command":"my-cli"}`,
				wantStatus: http.StatusBadRequest, wantError: "display name must produce a valid slug",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo := newFakeSettingsRepo()
				hub := &duplicateHub{}
				router := newSettingsRouter(t, repo, hub)
				response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/tui", tc.body)
				requireErrorBody(t, response, tc.wantStatus, tc.wantError)
				if len(repo.agents) != 0 || len(hub.actions()) != 0 {
					t.Fatalf("rejected create wrote %d agents and broadcast %v", len(repo.agents), hub.actions())
				}
			})
		}
	})

	t.Run("a second agent with the same slug is 409", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		router := newSettingsRouter(t, repo, &duplicateHub{})
		body := `{"display_name":"My CLI","command":"my-cli"}`

		if first := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/tui", body); first.Code != http.StatusOK {
			t.Fatalf("first create status = %d, body = %s", first.Code, first.Body.String())
		}
		second := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/tui", body)
		requireErrorBody(t, second, http.StatusConflict, "agent already exists")
		if len(repo.agents) != 1 {
			t.Fatalf("conflicting create left %d agent rows, want 1", len(repo.agents))
		}
	})
}

// TestMaintenanceErrorClassifiers pins the status/message each controller
// sentinel maps to. These feed both the install and the update endpoints, so a
// mis-mapped sentinel shows up as the wrong HTTP status on either route.
func TestMaintenanceErrorClassifiers(t *testing.T) {
	t.Run("install", func(t *testing.T) {
		cases := []struct {
			err         error
			wantStatus  int
			wantMessage string
			wantMatched bool
		}{
			{controller.ErrAgentNotFound, http.StatusNotFound, "agent not found", true},
			{controller.ErrInstallScriptEmpty, http.StatusBadRequest, "agent has no install script", true},
			{controller.ErrJobStoreUnavailable, http.StatusServiceUnavailable, "install service not ready", true},
			{errors.New("disk on fire"), 0, "", false},
		}
		for _, tc := range cases {
			status, message, matched := classifyInstallError(tc.err)
			if status != tc.wantStatus || message != tc.wantMessage || matched != tc.wantMatched {
				t.Errorf("classifyInstallError(%v) = (%d, %q, %v), want (%d, %q, %v)",
					tc.err, status, message, matched, tc.wantStatus, tc.wantMessage, tc.wantMatched)
			}
		}
	})

	t.Run("update", func(t *testing.T) {
		cases := []struct {
			err         error
			wantStatus  int
			wantMessage string
			wantMatched bool
		}{
			{controller.ErrAgentNotFound, http.StatusNotFound, "agent not found", true},
			{controller.ErrRuntimeUpdateUnsupported, http.StatusBadRequest, "agent runtime update unsupported", true},
			{controller.ErrRuntimeUpdaterUnavailable, http.StatusServiceUnavailable, "agent update service not ready", true},
			{controller.ErrRuntimeUpdateTargetRequired, http.StatusBadRequest, "target version is required", true},
			{controller.ErrRuntimeUpdateTargetInvalid, http.StatusBadRequest, "target version is invalid", true},
			{controller.ErrRuntimeUpdateTargetMissing, http.StatusBadRequest, "target version is not published", true},
			// Preview-only: the plain update classifier must not claim it, or
			// the enqueue path would answer 502 for an error it cannot get.
			{controller.ErrRuntimeUpdatePreviewFailed, 0, "", false},
			{errors.New("disk on fire"), 0, "", false},
		}
		for _, tc := range cases {
			status, message, matched := classifyUpdateError(tc.err)
			if status != tc.wantStatus || message != tc.wantMessage || matched != tc.wantMatched {
				t.Errorf("classifyUpdateError(%v) = (%d, %q, %v), want (%d, %q, %v)",
					tc.err, status, message, matched, tc.wantStatus, tc.wantMessage, tc.wantMatched)
			}
		}
	})

	t.Run("update preview adds the upstream-resolution failure", func(t *testing.T) {
		status, message, matched := classifyUpdatePreviewError(controller.ErrRuntimeUpdatePreviewFailed)
		if status != http.StatusBadGateway || message != "unable to resolve runtime versions" || !matched {
			t.Errorf("preview classifier = (%d, %q, %v), want (502, %q, true)",
				status, message, matched, "unable to resolve runtime versions")
		}
		// Everything the update classifier already handles is delegated.
		if status, message, matched := classifyUpdatePreviewError(controller.ErrAgentNotFound); //nolint:govet // shadow is intentional
		status != http.StatusNotFound || message != "agent not found" || !matched {
			t.Errorf("delegated classification = (%d, %q, %v)", status, message, matched)
		}
		if _, _, matched := classifyUpdatePreviewError(errors.New("disk on fire")); matched {
			t.Error("unrelated error must not be classified")
		}
	})
}
