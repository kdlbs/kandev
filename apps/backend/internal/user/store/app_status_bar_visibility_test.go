package store

import (
	"encoding/json"
	"testing"
)

func TestScanUserSettingsAppStatusBarVisibilityDefaultsAndRoundTrips(t *testing.T) {
	t.Setenv("KANDEV_FEATURES_APP_STATUS_BAR", "true")
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "missing defaults to disabled", raw: `{}`, want: false},
		{name: "explicit true is preserved", raw: `{"app_status_bar_enabled":true}`, want: true},
		{name: "explicit false is preserved", raw: `{"app_status_bar_enabled":false}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			encoded, err := marshalUserSettingsPayload(settings)
			if err != nil {
				t.Fatalf("marshal settings payload: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatalf("decode normalized settings: %v", err)
			}
			got, ok := payload["app_status_bar_enabled"].(bool)
			if !ok || got != tt.want {
				t.Fatalf("app_status_bar_enabled = %#v, want %t", payload["app_status_bar_enabled"], tt.want)
			}
		})
	}
}
