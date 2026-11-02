package config

import "testing"

// TestNewInstanceConfigPromptSuggestions verifies the per-instance prompt
// suggestion request reaches the instance config and defaults to off.
func TestNewInstanceConfigPromptSuggestions(t *testing.T) {
	base := &Config{}
	if base.NewInstanceConfig(41001, &InstanceOverrides{}).PromptSuggestions {
		t.Fatal("PromptSuggestions = true without an override, want false")
	}
	if !base.NewInstanceConfig(41001, &InstanceOverrides{PromptSuggestions: true}).PromptSuggestions {
		t.Fatal("PromptSuggestions = false with the override, want true")
	}
}
