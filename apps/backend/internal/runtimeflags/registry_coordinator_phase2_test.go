package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/profiles"
)

func TestCoordinatorPhase2FlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.coordinatorPhase2")
	if !ok {
		t.Fatal("features.coordinatorPhase2 definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_COORDINATOR_PHASE2" {
		t.Fatalf("EnvVar = %q", def.EnvVar)
	}
	if def.Label != "Coordinator control" || !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
	defaults, err := profiles.FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("profiles.FeatureFlagDefaults: %v", err)
	}
	if got := defaults["coordinator_phase2"]; got != "false" {
		t.Fatalf("prod default = %q, want false", got)
	}
}
