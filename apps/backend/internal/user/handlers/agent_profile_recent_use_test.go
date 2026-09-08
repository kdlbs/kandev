package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPAgentProfileRecentUseRoundTrip(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/user/agent-profile-recent-use/task_create",
		bytes.NewReader([]byte(`{"agent_profile_id":"profile-a"}`)),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT recent-use status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/user/agent-profile-recent-use",
		nil,
	))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET recent-use status = %d, want %d: %s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	var records []struct {
		Context    string   `json:"context"`
		ProfileIDs []string `json:"profile_ids"`
		Revision   int64    `json:"revision"`
	}
	if err := json.Unmarshal(getResponse.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode GET recent-use: %v", err)
	}
	if len(records) != 1 || records[0].Context != "task_create" || records[0].Revision != 1 {
		t.Fatalf("GET records = %+v, want one revision-one task-create record", records)
	}
	if len(records[0].ProfileIDs) != 1 || records[0].ProfileIDs[0] != "profile-a" {
		t.Fatalf("profile ids = %v, want [profile-a]", records[0].ProfileIDs)
	}
}

func TestHTTPAgentProfileRecentUseRejectsInvalidInput(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	for _, path := range []string{
		"/api/v1/user/agent-profile-recent-use/unknown",
		"/api/v1/user/agent-profile-recent-use/task_create",
	} {
		request := httptest.NewRequest(http.MethodPut, path, bytes.NewReader([]byte(`{"agent_profile_id":""}`)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s status = %d, want %d", path, response.Code, http.StatusBadRequest)
		}
	}
}
