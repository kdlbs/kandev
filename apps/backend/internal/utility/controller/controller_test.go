package controller

import (
	"testing"

	"github.com/kandev/kandev/internal/utility/service"
)

// TestWithFallbackProfile verifies the request's fallback profile reaches the
// service without mutating the caller's defaults.
func TestWithFallbackProfile(t *testing.T) {
	if got := withFallbackProfile(nil, ""); got != nil {
		t.Fatalf("no fallback and no defaults = %#v, want nil", got)
	}
	got := withFallbackProfile(nil, "session")
	if got == nil || got.FallbackProfileID != "session" || got.ProfileID != "" {
		t.Fatalf("fallback without defaults = %#v", got)
	}
	defaults := &service.DefaultUtilitySettings{ProfileID: "default"}
	merged := withFallbackProfile(defaults, "session")
	if merged.ProfileID != "default" || merged.FallbackProfileID != "session" {
		t.Fatalf("merged = %#v", merged)
	}
	if defaults.FallbackProfileID != "" {
		t.Fatal("withFallbackProfile mutated the caller's defaults")
	}
	if withFallbackProfile(defaults, "") != defaults {
		t.Fatal("empty fallback should return the defaults unchanged")
	}
}
