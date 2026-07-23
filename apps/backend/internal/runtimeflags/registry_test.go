package runtimeflags

import "testing"

func TestDefinitionsIncludeOfficeExperimentalMetadata(t *testing.T) {
	def, ok := DefinitionByKey("features.office")
	if !ok {
		t.Fatal("features.office definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_OFFICE" {
		t.Fatalf("EnvVar = %q, want KANDEV_FEATURES_OFFICE", def.EnvVar)
	}
	if def.Stability != StabilityExperimental {
		t.Fatalf("Stability = %q, want %q", def.Stability, StabilityExperimental)
	}
	if def.RiskDescription == "" {
		t.Fatal("RiskDescription empty")
	}
	if !def.RestartRequired {
		t.Fatal("RestartRequired = false, want true")
	}
}

func TestDefinitionsIncludePluginsExperimentalMetadata(t *testing.T) {
	def, ok := DefinitionByKey("features.plugins")
	if !ok {
		t.Fatal("features.plugins definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_PLUGINS" {
		t.Fatalf("EnvVar = %q, want KANDEV_FEATURES_PLUGINS", def.EnvVar)
	}
	if def.Stability != StabilityExperimental {
		t.Fatalf("Stability = %q, want %q", def.Stability, StabilityExperimental)
	}
	if def.RiskDescription == "" {
		t.Fatal("RiskDescription empty")
	}
	if !def.RestartRequired {
		t.Fatal("RestartRequired = false, want true")
	}
}

func TestDefinitionsIncludeAppStatusBarMetadata(t *testing.T) {
	def, ok := DefinitionByKey("features.appStatusBar")
	if !ok {
		t.Fatal("features.appStatusBar definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_APP_STATUS_BAR" {
		t.Fatalf("EnvVar = %q, want KANDEV_FEATURES_APP_STATUS_BAR", def.EnvVar)
	}
	if !def.RestartRequired {
		t.Fatal("RestartRequired = false, want true")
	}
}

func TestDefinitionsExposeSingleUserFacingDebugToggle(t *testing.T) {
	def, ok := DefinitionByKey("debug.devMode")
	if !ok {
		t.Fatal("debug.devMode definition missing")
	}
	if def.EnvVar != "KANDEV_DEBUG_DEV_MODE" {
		t.Fatalf("EnvVar = %q, want KANDEV_DEBUG_DEV_MODE", def.EnvVar)
	}
	if len(def.ImpliedEnvVars) == 0 {
		t.Fatal("Debug mode should imply subordinate debug env vars")
	}
	if _, ok := DefinitionByKey("debug.agentMessages"); ok {
		t.Fatal("debug.agentMessages must not be a top-level user-facing toggle")
	}
}
