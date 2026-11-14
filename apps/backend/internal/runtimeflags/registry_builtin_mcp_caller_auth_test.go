package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
	"gopkg.in/yaml.v3"
)

func TestBuiltinMCPCallerAuthFlagRegistration(t *testing.T) {
	def, ok := DefinitionByKey("features.builtinMcpCallerAuth")
	if !ok {
		t.Fatal("features.builtinMcpCallerAuth definition missing")
	}
	if def.EnvVar != "KANDEV_FEATURES_BUILTIN_MCP_CALLER_AUTH" {
		t.Fatalf("EnvVar = %q", def.EnvVar)
	}
	if !def.RestartRequired || !def.Mutable {
		t.Fatalf("unexpected metadata: %+v", def)
	}
	cfg := &config.Config{}
	for _, registration := range registrations {
		if registration.definition.Key == def.Key {
			registration.apply(cfg, true)
		}
	}
	if !cfg.Features.BuiltinMCPCallerAuth {
		t.Fatal("registration does not bind FeaturesConfig.BuiltinMCPCallerAuth")
	}
}

// The toggle ships off in prod and dev; e2e is the documented test-only
// exception so the enforcement path runs in CI.
func TestBuiltinMCPCallerAuthProfileDefaults(t *testing.T) {
	var doc struct {
		Features map[string]map[string]string `yaml:"features"`
	}
	if err := yaml.Unmarshal(profiles.ProfilesYAML(), &doc); err != nil {
		t.Fatalf("parse profiles.yaml: %v", err)
	}
	values := doc.Features["KANDEV_FEATURES_BUILTIN_MCP_CALLER_AUTH"]
	want := map[string]string{"prod": "false", "dev": "false", "e2e": "true"}
	for profile, value := range want {
		if values[profile] != value {
			t.Fatalf("%s default = %q, want %q", profile, values[profile], value)
		}
	}
}
