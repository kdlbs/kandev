package handlers

import (
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// The gin PATCH request struct must forward the provider triple to the
// controller. Without the wiring gin silently drops the unknown JSON fields, so
// reconfiguring or clearing a provider on an existing profile reports success
// while persisting nothing.
func TestUpdateProfileEndpoint_WiresProviderFields(t *testing.T) {
	repo := newFakeSettingsRepo()
	seedAgent(repo, "agent-1", "profile-agent", false)
	p := seedProfile(repo, "profile-1", "agent-1", "Router", "")
	p.ProviderKind = models.ProviderKindOpenAICompatible
	p.ProviderBaseURL = "http://localhost:20128/v1"
	router := newSettingsRouter(t, repo, nil)

	// Switching back to Native must clear the persisted provider fields; that
	// only happens if provider_kind actually reached the controller.
	response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1",
		`{"provider_kind":"","provider_base_url":"","provider_api_key_secret_id":""}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got dto.AgentProfileDTO
	decodeSettingsJSON(t, response, &got)
	if got.ProviderKind != models.ProviderKindNative || got.ProviderBaseURL != "" {
		t.Fatalf("response still carries provider config: %#v", got)
	}
	if len(repo.updatedProfiles) != 1 {
		t.Fatalf("persisted updates = %d, want 1", len(repo.updatedProfiles))
	}
	if repo.updatedProfiles[0].ProviderBaseURL != "" || repo.updatedProfiles[0].ProviderKind != models.ProviderKindNative {
		t.Fatalf("persisted profile kept provider config: %#v", repo.updatedProfiles[0])
	}
}
