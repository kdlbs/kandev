package handlers

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// newMcpFixture seeds one agent with one profile and returns the repo plus a
// router; supportsMCP toggles the agent's MCP capability.
func newMcpFixture(t *testing.T, supportsMCP bool) (*fakeSettingsRepo, *gin.Engine) {
	t.Helper()
	repo := newFakeSettingsRepo()
	seedAgent(repo, "agent-1", "mcp-agent", supportsMCP)
	seedProfile(repo, "profile-1", "agent-1", "Default", "")
	return repo, newSettingsRouter(t, repo, nil)
}

// TestGetProfileMcpConfigEndpoint pins the default-config body for a profile
// with no stored row, the stored body, and each distinct rejection: an unknown
// profile is 404 while an agent that does not support MCP is 400 — two
// different causes the UI renders differently.
func TestGetProfileMcpConfigEndpoint(t *testing.T) {
	t.Run("profile without a stored row gets the empty default config", func(t *testing.T) {
		_, router := newMcpFixture(t, true)

		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile-1/mcp-config", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		// `servers` must serialize as an object even when empty (the settings
		// page indexes into it); `meta` is omitempty, so an empty map is
		// absent from the body entirely.
		if body := response.Body.String(); body != `{"profile_id":"profile-1","enabled":false,"servers":{}}` {
			t.Fatalf("body = %s", body)
		}
		var got dto.AgentProfileMcpConfigDTO
		decodeSettingsJSON(t, response, &got)
		want := dto.AgentProfileMcpConfigDTO{
			ProfileID: "profile-1",
			Enabled:   false,
			Servers:   map[string]mcpconfig.ServerDef{},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("config = %#v, want %#v", got, want)
		}
	})

	t.Run("stored row is returned verbatim", func(t *testing.T) {
		repo, router := newMcpFixture(t, true)
		repo.mcpConfigs["profile-1"] = &models.AgentProfileMcpConfig{
			ProfileID: "profile-1",
			Enabled:   true,
			Servers: map[string]interface{}{
				"linear": map[string]interface{}{"type": "http", "url": "https://mcp.linear.app"},
			},
			Meta: map[string]interface{}{"source": "user"},
		}

		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile-1/mcp-config", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentProfileMcpConfigDTO
		decodeSettingsJSON(t, response, &got)
		want := dto.AgentProfileMcpConfigDTO{
			ProfileID: "profile-1",
			Enabled:   true,
			Servers: map[string]mcpconfig.ServerDef{
				"linear": {Type: "http", URL: "https://mcp.linear.app"},
			},
			Meta: map[string]any{"source": "user"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("config = %#v, want %#v", got, want)
		}
	})

	t.Run("unknown profile is 404", func(t *testing.T) {
		_, router := newMcpFixture(t, true)
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/missing/mcp-config", "")
		requireErrorBody(t, response, http.StatusNotFound, "agent profile not found")
	})

	t.Run("agent without MCP support is 400, not 404", func(t *testing.T) {
		_, router := newMcpFixture(t, false)
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile-1/mcp-config", "")
		requireErrorBody(t, response, http.StatusBadRequest, "mcp not supported by agent")
	})

	t.Run("store failure is 500", func(t *testing.T) {
		repo, router := newMcpFixture(t, true)
		repo.failWith("GetAgentProfileMcpConfig", errors.New("database is locked"))
		response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile-1/mcp-config", "")
		requireErrorBody(t, response, http.StatusInternalServerError, "failed to get mcp config")
	})
}

// TestUpdateProfileMcpConfigEndpoint covers the persisted write, the
// `mcpServers` compatibility alias, the never-nil servers map, and each
// rejection.
func TestUpdateProfileMcpConfigEndpoint(t *testing.T) {
	t.Run("persists the servers map and echoes it back", func(t *testing.T) {
		repo, router := newMcpFixture(t, true)

		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-profiles/profile-1/mcp-config",
			`{"enabled":true,"servers":{"linear":{"type":"http","url":"https://mcp.linear.app"}},"meta":{"source":"user"}}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentProfileMcpConfigDTO
		decodeSettingsJSON(t, response, &got)
		want := dto.AgentProfileMcpConfigDTO{
			ProfileID: "profile-1",
			Enabled:   true,
			Servers:   map[string]mcpconfig.ServerDef{"linear": {Type: "http", URL: "https://mcp.linear.app"}},
			Meta:      map[string]any{"source": "user"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("config = %#v, want %#v", got, want)
		}
		if len(repo.upsertedMcp) != 1 {
			t.Fatalf("upserted %d rows, want 1", len(repo.upsertedMcp))
		}
		stored := repo.upsertedMcp[0]
		if stored.ProfileID != "profile-1" || !stored.Enabled {
			t.Fatalf("stored row = %#v", stored)
		}
		if _, ok := stored.Servers["linear"]; !ok {
			t.Fatalf("stored servers = %#v, want a linear entry", stored.Servers)
		}
	})

	t.Run("mcpServers is accepted as an alias when servers is absent", func(t *testing.T) {
		repo, router := newMcpFixture(t, true)

		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-profiles/profile-1/mcp-config",
			`{"enabled":true,"mcpServers":{"aliased":{"type":"stdio","command":"run-me"}}}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentProfileMcpConfigDTO
		decodeSettingsJSON(t, response, &got)
		want := map[string]mcpconfig.ServerDef{"aliased": {Type: "stdio", Command: "run-me"}}
		if !reflect.DeepEqual(got.Servers, want) {
			t.Fatalf("servers = %#v, want %#v", got.Servers, want)
		}
		if len(repo.upsertedMcp) != 1 || len(repo.upsertedMcp[0].Servers) != 1 {
			t.Fatalf("stored rows = %#v", repo.upsertedMcp)
		}
	})

	t.Run("omitted servers persists an empty map, never nil", func(t *testing.T) {
		repo, router := newMcpFixture(t, true)

		response := doSettingsRequest(router, http.MethodPost,
			"/api/v1/agent-profiles/profile-1/mcp-config", `{"enabled":false}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentProfileMcpConfigDTO
		decodeSettingsJSON(t, response, &got)
		if got.Servers == nil || len(got.Servers) != 0 {
			t.Fatalf("servers = %#v, want an empty non-nil map", got.Servers)
		}
		if repo.upsertedMcp[0].Servers == nil {
			t.Fatal("stored servers map is nil")
		}
	})

	t.Run("rejections", func(t *testing.T) {
		cases := []struct {
			name       string
			profileID  string
			supportMCP bool
			body       string
			wantStatus int
			wantError  string
		}{
			{
				name: "malformed json", profileID: "profile-1", supportMCP: true, body: "{",
				wantStatus: http.StatusBadRequest, wantError: "invalid payload",
			},
			{
				name: "enabled omitted", profileID: "profile-1", supportMCP: true, body: `{"servers":{}}`,
				wantStatus: http.StatusBadRequest, wantError: "enabled is required",
			},
			{
				name: "unknown profile", profileID: "missing", supportMCP: true, body: `{"enabled":true}`,
				wantStatus: http.StatusNotFound, wantError: "agent profile not found",
			},
			{
				name: "agent without mcp support", profileID: "profile-1", supportMCP: false, body: `{"enabled":true}`,
				wantStatus: http.StatusBadRequest, wantError: "mcp not supported by agent",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo, router := newMcpFixture(t, tc.supportMCP)
				response := doSettingsRequest(router, http.MethodPost,
					"/api/v1/agent-profiles/"+tc.profileID+"/mcp-config", tc.body)
				requireErrorBody(t, response, tc.wantStatus, tc.wantError)
				if len(repo.upsertedMcp) != 0 {
					t.Fatalf("rejected update wrote %d rows", len(repo.upsertedMcp))
				}
			})
		}
	})

	t.Run("store failure is 500", func(t *testing.T) {
		repo, router := newMcpFixture(t, true)
		repo.failWith("UpsertAgentProfileMcpConfig", errors.New("database is locked"))
		response := doSettingsRequest(router, http.MethodPost,
			"/api/v1/agent-profiles/profile-1/mcp-config", `{"enabled":true}`)
		requireErrorBody(t, response, http.StatusInternalServerError, "failed to update mcp config")
	})
}
