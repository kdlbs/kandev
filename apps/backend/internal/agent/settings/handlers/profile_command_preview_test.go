package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestSavedProfileCommandPreview(t *testing.T) {
	for _, native := range []bool{false, true} {
		name := "acp"
		if native {
			name = "native"
		}
		t.Run(name, func(t *testing.T) {
			repo := newFakeSettingsRepo()
			repo.agents["agent"] = &models.Agent{ID: "agent", Name: "preview-agent"}
			repo.profiles["profile"] = &models.AgentProfile{ID: "profile", AgentID: "agent", CommandPrefix: "/private/bin/wrapper --token=synthetic-private-value", CLIPassthrough: native, CLIFlags: []models.CLIFlag{{Flag: "--private=synthetic-flag", Enabled: true}}}
			router, _, registry := newSettingsHarness(t, repo, nil)
			registerTestTUIAgent(t, registry, "preview-agent", 1)
			response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile/command-preview", "")
			require.Equal(t, http.StatusOK, response.Code)
			var result dto.CommandPreviewResponse
			decodeSettingsJSON(t, response, &result)
			require.NotNil(t, result.Composition)
			require.NotNil(t, result.Composition.Wrapper)
			require.Equal(t, "wrapper", result.Composition.Wrapper.Command.Executable)
			require.Empty(t, result.Command)
			require.Empty(t, result.CommandString)
			for _, private := range []string{"/private", "synthetic-private-value", "synthetic-flag"} {
				require.NotContains(t, response.Body.String(), private)
			}
			digest := result.Composition.Wrapper.Digest
			repo.profiles["profile"].CommandPrefix = "another-wrapper --"
			changed := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile/command-preview", "")
			require.Equal(t, http.StatusOK, changed.Code)
			decodeSettingsJSON(t, changed, &result)
			require.NotEqual(t, digest, result.Composition.Wrapper.Digest)
		})
	}
}

func TestSavedProfileCommandPreviewMissingOrUnusable(t *testing.T) {
	for _, kind := range []string{"missing", "dynamic", "workspace", "invalid-wrapper"} {
		t.Run(kind, func(t *testing.T) {
			repo := newFakeSettingsRepo()
			repo.agents["agent"] = &models.Agent{ID: "agent", Name: "preview-agent"}
			profile := &models.AgentProfile{ID: "profile", AgentID: "agent"}
			repo.profiles[profile.ID] = profile
			switch kind {
			case "missing":
				delete(repo.profiles, profile.ID)
			case "dynamic":
				profile.AgentID = agents.DynamicAgentID
			case "workspace":
				profile.WorkspaceID = "private-workspace"
			case "invalid-wrapper":
				profile.CommandPrefix = "wrapper 'synthetic-private-value"
			}
			router, _, registry := newSettingsHarness(t, repo, nil)
			registerTestTUIAgent(t, registry, "preview-agent", 1)
			response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile/command-preview", "")
			require.NotEqual(t, http.StatusOK, response.Code)
			require.NotContains(t, response.Body.String(), "synthetic-private-value")
			require.NotContains(t, response.Body.String(), "private-workspace")
		})
	}
}

func TestSavedProfileCommandPreviewRequiresConfigurationAuthority(t *testing.T) {
	router := settingsRouterAs(t, authn.Identity{UserID: "member", Role: authn.RoleMember})
	response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile/command-preview", "")
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Contains(t, response.Body.String(), "org.config.manage")
}

func TestSavedProfileCommandPreviewSanitizesStoreFailures(t *testing.T) {
	for _, operation := range []string{"GetAgentProfile", "GetAgent"} {
		t.Run(operation, func(t *testing.T) {
			repo := newFakeSettingsRepo()
			repo.profiles["profile"] = &models.AgentProfile{ID: "profile", AgentID: "agent"}
			repo.errs[operation] = errors.New("synthetic-private-value")
			router, _, _ := newSettingsHarness(t, repo, nil)
			response := doSettingsRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile/command-preview", "")
			require.Equal(t, http.StatusInternalServerError, response.Code)
			require.NotContains(t, response.Body.String(), "synthetic-private-value")
		})
	}
}
