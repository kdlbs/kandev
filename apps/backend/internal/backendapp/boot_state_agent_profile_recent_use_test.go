package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	usermodels "github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/webapp"
)

func TestBootInitialStateIncludesAgentProfileRecentUseSeparately(t *testing.T) {
	harness := newBootStateTestHarness(t)
	if _, err := harness.userSvc.RecordAgentProfileRecentUse(
		t.Context(),
		usermodels.AgentProfileRecentUseQuickChat,
		"profile-a",
	); err != nil {
		t.Fatalf("record recent-use profile: %v", err)
	}

	params := routeParams{userCtrl: harness.userCtrl}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	state := bootInitialState(t.Context(), request, params, webapp.RouteClassification{Route: webapp.RouteHome})
	recentUse, ok := state["agentProfileRecentUse"].(map[string]any)
	if !ok {
		t.Fatalf("agentProfileRecentUse = %#v, want separate state", state["agentProfileRecentUse"])
	}
	if loaded, ok := recentUse["loaded"].(bool); !ok || !loaded {
		t.Fatalf("agentProfileRecentUse.loaded = %#v, want true", recentUse["loaded"])
	}
	records, ok := recentUse["records"].(map[string]any)
	if !ok {
		t.Fatalf("agentProfileRecentUse.records = %#v, want context map", recentUse["records"])
	}
	record, ok := records[string(usermodels.AgentProfileRecentUseQuickChat)].(map[string]any)
	if !ok {
		t.Fatalf("quick-chat recent-use record = %#v", records[string(usermodels.AgentProfileRecentUseQuickChat)])
	}
	if ids, ok := record["profileIds"].([]string); !ok || len(ids) != 1 || ids[0] != "profile-a" {
		t.Fatalf("profileIds = %#v, want [profile-a]", record["profileIds"])
	}
	if _, embedded := state["userSettings"].(map[string]any); embedded {
		t.Fatal("recent-use state must not be embedded in userSettings")
	}
}
