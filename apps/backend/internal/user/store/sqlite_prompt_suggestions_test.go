package store

import "testing"

// TestScanUserSettingsPromptSuggestionsDefault verifies both prompt suggestion flags default to false and honor explicit values.
func TestScanUserSettingsPromptSuggestionsDefault(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantMain     bool
		wantFallback bool
	}{
		{name: "empty settings disable suggestions", raw: `{}`},
		{name: "missing settings disable suggestions", raw: `{"chat_submit_key":"enter"}`},
		{name: "explicit main only", raw: `{"prompt_suggestions":true}`, wantMain: true},
		{name: "explicit both", raw: `{"prompt_suggestions":true,"prompt_suggestions_fallback":true}`, wantMain: true, wantFallback: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			if settings.PromptSuggestions != tt.wantMain || settings.PromptSuggestionsFallback != tt.wantFallback {
				t.Fatalf("flags = (%v, %v), want (%v, %v)",
					settings.PromptSuggestions, settings.PromptSuggestionsFallback, tt.wantMain, tt.wantFallback)
			}
		})
	}
}
