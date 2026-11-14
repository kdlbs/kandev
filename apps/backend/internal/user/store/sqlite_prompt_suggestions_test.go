package store

import "testing"

// TestScanUserSettingsPromptSuggestionsDefault verifies the prompt suggestion flag defaults to false and honors explicit values.
func TestScanUserSettingsPromptSuggestionsDefault(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "empty settings disable suggestions", raw: `{}`},
		{name: "missing settings disable suggestions", raw: `{"chat_submit_key":"enter"}`},
		{name: "explicit on", raw: `{"prompt_suggestions":true}`, want: true},
		{name: "explicit off", raw: `{"prompt_suggestions":false}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			if settings.PromptSuggestions != tt.want {
				t.Fatalf("PromptSuggestions = %v, want %v", settings.PromptSuggestions, tt.want)
			}
		})
	}
}
