package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHTTPUpdateThreadViewsRoundTrip verifies that Threads view settings use
// the existing user-settings endpoint and survive a subsequent read.
func TestHTTPUpdateThreadViewsRoundTrip(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	patch := []byte(`{
		"thread_views":[{
			"id":"view-needs-attention",
			"name":"Needs attention",
			"task_scope":{"mode":"selected","task_ids":["task-a","task-b"]},
			"filters":[],
			"sort":{"key":"attention","direction":"asc"},
			"max_columns":3
		}],
		"thread_active_view_id":"view-needs-attention",
		"thread_view_draft":{
			"base_view_id":"view-needs-attention",
			"task_scope":{"mode":"all","task_ids":[]},
			"filters":[],
			"sort":{"key":"attention","direction":"asc"},
			"max_columns":null
		}
	}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader(patch))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PATCH thread views status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/api/v1/user/settings", nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET user settings status = %d, want %d: %s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(getResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode user settings: %v", err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(payload["settings"], &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if string(settings["thread_views"]) == "" || string(settings["thread_views"]) == "null" {
		t.Fatalf("thread_views = %s, want persisted view", settings["thread_views"])
	}
	if string(settings["thread_active_view_id"]) != `"view-needs-attention"` {
		t.Fatalf("thread_active_view_id = %s, want view-needs-attention", settings["thread_active_view_id"])
	}
	if string(settings["thread_view_draft"]) == "null" || string(settings["thread_view_draft"]) == "" {
		t.Fatalf("thread_view_draft = %s, want persisted draft", settings["thread_view_draft"])
	}
}

// TestHTTPThreadViewPatchPreservesOmittedFields verifies that a Threads-only
// patch does not replace the saved view collection or active selection.
func TestHTTPThreadViewPatchPreservesOmittedFields(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	initial := []byte(`{
		"thread_views":[{"id":"view-one","name":"One","task_scope":{"mode":"all","task_ids":[]},"filters":[],"sort":{"key":"attention","direction":"asc"},"max_columns":null}],
		"thread_active_view_id":"view-one"
	}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader(initial))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("initial PATCH status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader([]byte(`{"thread_active_view_id":"view-one"}`)))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("follow-up PATCH status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var responsePayload struct {
		Settings struct {
			ThreadViews []json.RawMessage `json:"thread_views"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &responsePayload); err != nil {
		t.Fatalf("decode PATCH response: %v", err)
	}
	if len(responsePayload.Settings.ThreadViews) != 1 {
		t.Fatalf("thread view count = %d, want 1", len(responsePayload.Settings.ThreadViews))
	}
}

func TestHTTPUpdateThreadViewsRejectsEmptySelectedScope(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	patch := []byte(`{
		"thread_views":[{
			"id":"view-empty-selected",
			"name":"Empty selected",
			"task_scope":{"mode":"selected","task_ids":[]},
			"filters":[],
			"sort":{"key":"attention","direction":"asc"},
			"max_columns":null
		}],
		"thread_active_view_id":"view-empty-selected"
	}`)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/user/settings", bytes.NewReader(patch))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("PATCH empty selected scope status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}
