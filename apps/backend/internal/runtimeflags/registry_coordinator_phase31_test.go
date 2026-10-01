package runtimeflags

import "testing"

func TestCoordinatorPhase31FlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.coordinatorPhase31")
	if !ok {
		t.Fatal("features.coordinatorPhase31 definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_COORDINATOR_PHASE31" {
		t.Fatalf("EnvVar = %q", def.EnvVar)
	}
	if !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
}
