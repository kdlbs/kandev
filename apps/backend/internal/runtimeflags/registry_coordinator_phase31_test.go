package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/profiles"
)

func TestCoordinatorPhase31FlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.coordinatorPhase31")
	if !ok {
		t.Fatal("features.coordinatorPhase31 definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_COORDINATOR_PHASE31" || !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
	defaults, err := profiles.FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("profiles.FeatureFlagDefaults: %v", err)
	}
	if got := defaults["coordinator_phase31"]; got != "false" {
		t.Fatalf("prod default = %q, want false", got)
	}
}
