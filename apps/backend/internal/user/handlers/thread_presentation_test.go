package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func threadSettingsRequest(t *testing.T, router *gin.Engine, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/user/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func readThreadPresentation(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Settings
}

// @covers AC-UI-THREADS-SAVED-VIEWS-005.1, AC-UI-THREADS-SAVED-VIEWS-005.2
func TestHTTPThreadPresentationRoundTripAndOmittedCollections(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	patch := `{"thread_views":[{"id":"one","name":"One","task_scope":{"mode":"all","task_ids":[]},"filters":[],"sort":{"key":"priority","direction":"desc"},"max_columns":7,"layout":"grid","auto_hide_composer":true}],"thread_active_view_id":"one","thread_view_draft":{"base_view_id":"one","task_scope":{"mode":"all","task_ids":[]},"max_columns":null,"layout":"columns","auto_hide_composer":true}}`
	readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodPatch, patch))
	readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodPatch, `{"thread_active_view_id":"one"}`))
	settings := readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodGet, ""))
	view := settings["thread_views"].([]any)[0].(map[string]any)
	draft := settings["thread_view_draft"].(map[string]any)
	if view["layout"] != "grid" || view["auto_hide_composer"] != true || view["max_columns"] != float64(7) {
		t.Fatalf("view = %+v", view)
	}
	if draft["layout"] != "columns" || draft["auto_hide_composer"] != true || draft["max_columns"] != nil {
		t.Fatalf("draft = %+v", draft)
	}
}

// @covers AC-UI-THREADS-SAVED-VIEWS-005.3
func TestHTTPThreadPresentationRejectsInvalidWritesAtomically(t *testing.T) {
	for _, fields := range []string{
		`"layout":"masonry"`, `"layout":{}`, `"auto_hide_composer":"true"`,
		`"layout":""`, `"layout":null`, `"auto_hide_composer":null`,
	} {
		t.Run(fields, func(t *testing.T) {
			router := newTestUserSettingsRouter(t)
			before := threadSettingsRequest(t, router, http.MethodGet, "").Body.String()
			patch := `{"thread_views":[{"id":"new","name":"Invalid","task_scope":{"mode":"all","task_ids":[]},` + fields + `}],"thread_active_view_id":"new"}`
			response := threadSettingsRequest(t, router, http.MethodPatch, patch)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
			}
			after := threadSettingsRequest(t, router, http.MethodGet, "").Body.String()
			if before != after {
				t.Fatal("rejected write changed persisted settings")
			}
		})
	}
}
