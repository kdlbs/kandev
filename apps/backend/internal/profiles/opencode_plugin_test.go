package profiles

import "testing"

func TestOpenCodePluginIsOffInEveryShippedProfile(t *testing.T) {
	defaults, err := FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("FeatureFlagDefaults: %v", err)
	}
	if got := defaults["opencode_plugin"]; got != "false" {
		t.Fatalf("profiles.yaml opencode_plugin default = %q, want false", got)
	}
}
