package handlers

import (
	"net/http"
	"testing"
)

// @covers AC-UI-NAV-HIERARCHY-004.2, AC-UI-NAV-HIERARCHY-004.6
func TestSidebarPresentationSettingsRoundTrip(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	read := func() map[string]any {
		return readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodGet, ""))
	}
	initial := read()
	if initial["sidebar_fast_actions_enabled"] != false || initial["sidebar_new_task_style"] != "simple" {
		t.Fatalf("new user defaults: %v", initial)
	}
	readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodPatch, `{"sidebar_fast_actions_enabled":true,"sidebar_new_task_style":"compact"}`))
	readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodPatch, `{"app_status_bar_enabled":true}`))
	got := read()
	if got["sidebar_fast_actions_enabled"] != true || got["sidebar_new_task_style"] != "compact" {
		t.Fatalf("settings were not retained: %v", got)
	}
	readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodPatch, `{"sidebar_fast_actions_enabled":false}`))
	if got := read(); got["sidebar_fast_actions_enabled"] != false || got["sidebar_new_task_style"] != "compact" {
		t.Fatalf("independent patch: %v", got)
	}
}

func TestSidebarPresentationRejectsInvalidAtomicWrites(t *testing.T) {
	for _, body := range []string{`{"sidebar_fast_actions_enabled":null}`, `{"sidebar_fast_actions_enabled":"false"}`, `{"sidebar_new_task_style":null}`, `{"sidebar_new_task_style":"unknown","sidebar_fast_actions_enabled":true}`} {
		t.Run(body, func(t *testing.T) {
			router := newTestUserSettingsRouter(t)
			before := threadSettingsRequest(t, router, http.MethodGet, "").Body.String()
			response := threadSettingsRequest(t, router, http.MethodPatch, body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("accepted invalid patch: %d %s", response.Code, response.Body.String())
			}
			if after := threadSettingsRequest(t, router, http.MethodGet, "").Body.String(); after != before {
				t.Fatal("invalid request changed settings")
			}
		})
	}
}
