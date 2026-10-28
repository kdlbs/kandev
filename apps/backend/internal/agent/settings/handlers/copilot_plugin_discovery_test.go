package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
)

type handlerCopilotRunner struct {
	stdout []byte
}

func (r handlerCopilotRunner) Run(context.Context, []string, map[string]string, string) (copilotplugins.CommandResult, error) {
	return copilotplugins.CommandResult{Stdout: r.stdout}, nil
}

func TestCopilotPluginDiscoveryRoute(t *testing.T) {
	repo := newFakeSettingsRepo()
	agent := repo.putAgent(&models.Agent{ID: "agent-row-1", Name: agents.CopilotACPAgentID, SupportsMCP: true})
	repo.putProfile(&models.AgentProfile{ID: "profile-1", AgentID: agent.ID, Name: "Work"})
	router, ctrl, reg := newSettingsHarness(t, repo, nil)
	if err := reg.Register(agents.NewCopilotACP()); err != nil {
		t.Fatal(err)
	}
	ctrl.SetCopilotPluginRunner(handlerCopilotRunner{stdout: []byte(`[{"name":"spark","marketplace":"copilot-plugins","version":"1.0.0","enabled":true,"source":"installed"}]`)})
	response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/agent-row-1/plugin-discovery", `{"profile_id":"profile-1"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got dto.CopilotPluginDiscoveryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "ready" || got.ProfileID != "profile-1" || len(got.Plugins) != 1 {
		t.Fatalf("response = %#v", got)
	}
	if strings.Contains(response.Body.String(), "cache_path") {
		t.Fatalf("response leaked native metadata: %s", response.Body.String())
	}
}

func TestCopilotPluginDiscoveryRouteRequiresSettingsAuthority(t *testing.T) {
	repo := newFakeSettingsRepo()
	repo.putAgent(&models.Agent{ID: "agent-row-1", Name: agents.CopilotACPAgentID})
	router, _, _ := newSettingsHarnessAs(t, repo, nil, authn.Identity{UserID: "member", Role: authn.Role("member")})
	response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/agent-row-1/plugin-discovery", `{}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestCopilotPluginDiscoveryRouteRejectsUnknownFields(t *testing.T) {
	repo := newFakeSettingsRepo()
	repo.putAgent(&models.Agent{ID: "agent-row-1", Name: agents.CopilotACPAgentID})
	router, _, _ := newSettingsHarness(t, repo, nil)
	response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/agent-row-1/plugin-discovery", `{"plugin_dir":"/tmp/unsafe"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
