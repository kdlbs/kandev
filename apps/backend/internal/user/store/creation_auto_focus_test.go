package store

import (
	"encoding/json"
	"testing"
)

// @covers AC-TASKS-CREATION-AUTO-FOCUS-001.1, AC-TASKS-CREATION-AUTO-FOCUS-001.4
func TestAutoFocusNewTasksStoredDefaultAndRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{"legacy empty", `{}`, true},
		{"legacy other fields", `{"chat_submit_key":"cmd_enter"}`, true},
		{"disabled", `{"auto_focus_new_tasks":false}`, false},
		{"enabled", `{"auto_focus_new_tasks":true}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tc.raw}, DefaultUserID)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := marshalUserSettingsPayload(settings)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			if got := payload["auto_focus_new_tasks"]; got != tc.want {
				t.Fatalf("auto_focus_new_tasks = %#v, want %v", got, tc.want)
			}
		})
	}
}
