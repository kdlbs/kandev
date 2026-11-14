package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

func TestCoordinatorPhase35FlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.coordinatorPhase35")
	if !ok {
		t.Fatal("features.coordinatorPhase35 definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_COORDINATOR_PHASE35" {
		t.Fatalf("EnvVar = %q", def.EnvVar)
	}
	if !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
	defaults, err := profiles.FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("profiles.FeatureFlagDefaults: %v", err)
	}
	if got := defaults["coordinator_phase35"]; got != "false" {
		t.Fatalf("prod default = %q, want false", got)
	}
}

func TestCoordinatorPhase35FlagConfigBinding(t *testing.T) {
	var cfg config.Config
	cfg.Features.CoordinatorPhase35 = true
	if !ValuesFromConfig(&cfg)["features.coordinatorPhase35"] {
		t.Fatal("ValuesFromConfig does not read CoordinatorPhase35")
	}
}
