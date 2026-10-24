package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/profiles"
)

func TestCoordinatorPhase3FlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.coordinatorPhase3")
	if !ok {
		t.Fatal("features.coordinatorPhase3 definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_COORDINATOR_PHASE3" {
		t.Fatalf("EnvVar = %q", def.EnvVar)
	}
	if def.Label != "Coordinator autonomy" || !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
	defaults, err := profiles.FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("profiles.FeatureFlagDefaults: %v", err)
	}
	if got := defaults["coordinator_phase3"]; got != "false" {
		t.Fatalf("prod default = %q, want false", got)
	}
}
