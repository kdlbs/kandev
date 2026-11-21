package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

func TestCoordinatorAnswerFlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.coordinatorAnswer")
	if !ok {
		t.Fatal("features.coordinatorAnswer definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_COORDINATOR_ANSWER" {
		t.Fatalf("EnvVar = %q", def.EnvVar)
	}
	if !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
	defaults, err := profiles.FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("profiles.FeatureFlagDefaults: %v", err)
	}
	if got := defaults["coordinator_answer"]; got != "false" {
		t.Fatalf("prod default = %q, want false", got)
	}
}

func TestCoordinatorAnswerFlagConfigBinding(t *testing.T) {
	var cfg config.Config
	cfg.Features.CoordinatorAnswer = true
	if !ValuesFromConfig(&cfg)["features.coordinatorAnswer"] {
		t.Fatal("ValuesFromConfig does not read CoordinatorAnswer")
	}
}
