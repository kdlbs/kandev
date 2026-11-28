package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestExecutionGradeHTTPValidation(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/agents/agent-1/profiles", `{"name":"Profile","execution_grade":" bad"}`},
		{http.MethodPatch, "/api/v1/agent-profiles/profile-1", `{"execution_grade":"bad\nname"}`},
		{http.MethodPost, "/api/v1/agents", `{"name":"test-agent","profiles":[{"name":"Profile","execution_grade":" bad"}]}`},
	} {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "test-agent", false)
		seedProfile(repo, "profile-1", "agent-1", "Original", "")
		router, _, reg := newSettingsHarness(t, repo, nil)
		registerTestTUIAgent(t, reg, "test-agent", 1)
		response := doSettingsRequest(router, tc.method, tc.path, tc.body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
		}
		if len(repo.updatedProfiles) != 0 || len(repo.agents) != 1 || len(repo.profiles) != 1 {
			t.Fatal("invalid grade wrote settings")
		}
	}
}

func TestExecutionGradeNestedHTTPCreate(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	repo := newFakeSettingsRepo()
	router, _, reg := newSettingsHarness(t, repo, nil)
	registerTestTUIAgent(t, reg, "test-agent", 1)
	response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents", `{"name":"test-agent","profiles":[{"name":"Profile","execution_grade":"Tier-A"}]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	profiles, ok := got["profiles"].([]any)
	if !ok || len(profiles) != 1 {
		t.Fatalf("profiles=%v", got["profiles"])
	}
	if profiles[0].(map[string]any)["execution_grade"] != "Tier-A" {
		t.Fatalf("nested grade lost: %s", response.Body.String())
	}
}

func TestExecutionGradeHTTPResponseAndBroadcast(t *testing.T) {
	repo := newFakeSettingsRepo()
	seedAgent(repo, "agent-1", "test-agent", false)
	hub := &duplicateHub{}
	router, _, reg := newSettingsHarness(t, repo, hub)
	registerTestTUIAgent(t, reg, "test-agent", 1)
	created := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/agent-1/profiles", `{"name":"Profile","execution_grade":"Initial"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	id, ok := body["id"].(string)
	if !ok {
		t.Fatalf("id=%v", body["id"])
	}
	response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/"+id, `{"execution_grade":"Updated"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("patch=%d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	event := decodeProfileBroadcast(t, hub, ws.ActionAgentProfileUpdated)
	if body["execution_grade"] != "Updated" || event.ExecutionGrade != "Updated" || repo.profiles[id].ExecutionGrade != "Updated" {
		t.Fatalf("response/event/storage lost grade: %s / %q", response.Body.String(), event.ExecutionGrade)
	}
}
