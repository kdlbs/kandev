package store

import (
	"encoding/json"
	"testing"
)

func TestSidebarHoverStoredDefaults(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		enabled bool
		delay   float64
	}{
		{`{}`, true, 500},
		{`{"sidebar_hover_delay_ms":"invalid"}`, true, 500},
		{`{"sidebar_hover_delay_ms":0.5}`, true, 500},
		{`{"sidebar_hover_delay_ms":null}`, true, 500},
		{`{"sidebar_hover_enabled":false,"sidebar_hover_delay_ms":0}`, false, 0},
		{`{"sidebar_hover_delay_ms":1200}`, true, 1200},
		{`{"sidebar_hover_delay_ms":-1}`, true, 500},
		{`{"sidebar_hover_delay_ms":5001}`, true, 500},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tc.raw}, DefaultUserID)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := marshalUserSettingsPayload(settings)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err = json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got["sidebar_hover_enabled"] != tc.enabled || got["sidebar_hover_delay_ms"] != tc.delay {
				t.Fatalf("hover stored = %v, %v", got["sidebar_hover_enabled"], got["sidebar_hover_delay_ms"])
			}
		})
	}
}
