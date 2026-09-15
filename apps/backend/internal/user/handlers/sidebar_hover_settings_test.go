package handlers

import (
	"net/http"
	"testing"
)

// @covers AC-UI-SIDEBAR-HOVER-002.1, AC-UI-SIDEBAR-HOVER-002.2, AC-UI-SIDEBAR-HOVER-002.3
func TestSidebarHoverSettingsRoundTrip(t *testing.T) {
	router := newTestUserSettingsRouter(t)
	read := func() map[string]any {
		return readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodGet, ""))
	}
	defaults := read()
	if defaults["sidebar_hover_enabled"] != true || defaults["sidebar_hover_delay_ms"] != float64(500) {
		t.Fatalf("hover defaults = %v, %v", defaults["sidebar_hover_enabled"], defaults["sidebar_hover_delay_ms"])
	}
	for _, body := range []string{`{"sidebar_hover_enabled":false,"sidebar_hover_delay_ms":0}`, `{"app_status_bar_enabled":true}`} {
		readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodPatch, body))
	}
	got := read()
	if got["sidebar_hover_enabled"] != false || got["sidebar_hover_delay_ms"] != float64(0) {
		t.Fatalf("hover round trip = %v, %v", got["sidebar_hover_enabled"], got["sidebar_hover_delay_ms"])
	}
	readThreadPresentation(t, threadSettingsRequest(t, router, http.MethodPatch, `{"sidebar_hover_delay_ms":5000}`))
	got = read()
	if got["sidebar_hover_enabled"] != false || got["sidebar_hover_delay_ms"] != float64(5000) {
		t.Fatal("delay patch reset toggle or did not persist")
	}
}

func TestSidebarHoverSettingsRejectInvalidWrites(t *testing.T) {
	for _, value := range []string{"-1", "5001", "1.5", `""`, `"500"`} {
		t.Run(value, func(t *testing.T) {
			router := newTestUserSettingsRouter(t)
			before := threadSettingsRequest(t, router, http.MethodGet, "").Body.String()
			response := threadSettingsRequest(t, router, http.MethodPatch, `{"sidebar_hover_enabled":false,"sidebar_hover_delay_ms":`+value+`}`)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			after := threadSettingsRequest(t, router, http.MethodGet, "").Body.String()
			if before != after {
				t.Fatal("invalid request changed saved settings")
			}
		})
	}
}
