package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

func TestCoordinatorMergeFlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.coordinatorMerge")
	if !ok {
		t.Fatal("features.coordinatorMerge definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_COORDINATOR_MERGE" {
		t.Fatalf("EnvVar = %q", def.EnvVar)
	}
	if !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
	defaults, err := profiles.FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("profiles.FeatureFlagDefaults: %v", err)
	}
	if got := defaults["coordinator_merge"]; got != "false" {
		t.Fatalf("prod default = %q, want false", got)
	}
}

func TestCoordinatorMergeFlagConfigBinding(t *testing.T) {
	var cfg config.Config
	cfg.Features.CoordinatorMerge = true
	if !ValuesFromConfig(&cfg)["features.coordinatorMerge"] {
		t.Fatal("ValuesFromConfig does not read CoordinatorMerge")
	}
}
