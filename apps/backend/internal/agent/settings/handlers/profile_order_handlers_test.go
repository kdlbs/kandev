package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/httpmw"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestReorderProfilesEndpointSavesAndBroadcastsChangedOrder(t *testing.T) {
	repo := newFakeSettingsRepo()
	repo.putAgent(&models.Agent{ID: "agent-1", Name: "test-agent"})
	repo.profiles["first"] = &models.AgentProfile{ID: "first", AgentID: "agent-1", Name: "First"}
	repo.profiles["second"] = &models.AgentProfile{ID: "second", AgentID: "agent-1", Name: "Second"}
	hub := &duplicateHub{}
	router := newSettingsRouter(t, repo, hub)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/agents/agent-1/profiles/order", strings.NewReader(`{"profile_ids":["second","first"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body %s, want 200", response.Code, response.Body.String())
	}
	var body struct {
		ProfileIDs []string `json:"profile_ids"`
		Revision   int64    `json:"revision"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.ProfileIDs) != 2 || body.ProfileIDs[0] != "second" || body.ProfileIDs[1] != "first" || body.Revision != 1 {
		t.Fatalf("response = %+v, want reordered IDs and revision 1", body)
	}
	if len(hub.msgs) != 1 || hub.msgs[0].Action != ws.ActionAgentProfilesReordered {
		t.Fatalf("broadcasts = %#v, want one agent.profiles.reordered event", hub.msgs)
	}
	sameRequest := httptest.NewRequest(http.MethodPut, "/api/v1/agents/agent-1/profiles/order", strings.NewReader(`{"profile_ids":["second","first"]}`))
	sameRequest.Header.Set("Content-Type", "application/json")
	sameRequest.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	sameResponse := httptest.NewRecorder()
	router.ServeHTTP(sameResponse, sameRequest)
	if sameResponse.Code != http.StatusOK || len(hub.msgs) != 1 {
		t.Fatalf("unchanged reorder status %d broadcasts %d, want 200 and one total event", sameResponse.Code, len(hub.msgs))
	}
	staleRequest := httptest.NewRequest(http.MethodPut, "/api/v1/agents/agent-1/profiles/order", strings.NewReader(`{"profile_ids":["foreign"]}`))
	staleRequest.Header.Set("Content-Type", "application/json")
	staleRequest.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	staleResponse := httptest.NewRecorder()
	router.ServeHTTP(staleResponse, staleRequest)
	if staleResponse.Code != http.StatusConflict || !strings.Contains(staleResponse.Body.String(), "profile_order_stale") || len(hub.msgs) != 1 {
		t.Fatalf("stale reorder status %d body %s broadcasts %d, want 409 profile_order_stale and no event", staleResponse.Code, staleResponse.Body.String(), len(hub.msgs))
	}
}
