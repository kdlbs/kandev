package config

import "testing"

// TestNewInstanceConfigPromptSuggestions verifies the per-instance prompt
// suggestion request reaches the instance config and defaults to off.
func TestNewInstanceConfigPromptSuggestions(t *testing.T) {
	base := &Config{}
	instance, err := base.NewInstanceConfig(41001, &InstanceOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if instance.PromptSuggestions {
		t.Fatal("PromptSuggestions = true without an override, want false")
	}
	instance, err = base.NewInstanceConfig(41001, &InstanceOverrides{PromptSuggestions: true})
	if err != nil {
		t.Fatal(err)
	}
	if !instance.PromptSuggestions {
		t.Fatal("PromptSuggestions = false with the override, want true")
	}
}
