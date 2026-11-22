package runtimeflags

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

func TestDynamicCircuitBackoffFlagIsRetiredAndStaleValuesAreInert(t *testing.T) {
	const key = "features.dynamicCircuitBackoffEscalation"
	const envVar = "KANDEV_FEATURES_DYNAMIC_CIRCUIT_BACKOFF_ESCALATION"
	const mainKey = "features.dynamicAgentRouting"
	const mainEnv = "KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING"

	if _, ok := DefinitionByKey(key); ok {
		t.Fatalf("retired runtime flag %q remains active", key)
	}
	for _, definition := range Definitions() {
		if definition.EnvVar == envVar {
			t.Fatalf("active runtime flag %q still binds %s", definition.Key, envVar)
		}
	}
	if _, ok := ValuesFromConfig(&config.Config{})[key]; ok {
		t.Fatalf("retired runtime flag %q remains in config values", key)
	}
	retired := false
	for _, identity := range retiredRuntimeFlagIdentities {
		if identity.key == key && identity.envVar == envVar {
			retired = true
			break
		}
	}
	if !retired {
		t.Fatalf("retired identity pair %q / %q is missing", key, envVar)
	}

	root := runtimeflagsRepoRoot(t)
	for _, relPath := range []string{
		"profiles.yaml",
		"apps/backend/internal/profiles/profiles.yaml",
	} {
		content, err := os.ReadFile(filepath.Join(root, relPath))
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		if strings.Contains(string(content), envVar) {
			t.Fatalf("retired environment variable %q remains in %s", envVar, relPath)
		}
	}

	for _, profile := range []struct {
		name, dev, e2e string
	}{
		{name: "prod", dev: "false", e2e: "false"},
		{name: "dev", dev: "true", e2e: "false"},
		{name: "e2e", dev: "false", e2e: "true"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			t.Setenv("KANDEV_DEBUG_DEV_MODE", profile.dev)
			t.Setenv("KANDEV_E2E_MOCK", profile.e2e)
			defaults, err := profiles.FeatureFlagDefaults()
			if err != nil {
				t.Fatalf("profiles.FeatureFlagDefaults: %v", err)
			}
			if got := defaults["dynamic_agent_routing"]; got != "false" {
				t.Fatalf("dynamic agent routing default = %q in %s, want false", got, profile.name)
			}
			if _, ok := defaults["dynamic_circuit_backoff_escalation"]; ok {
				t.Fatalf("retired flag remains in %s profile defaults", profile.name)
			}
		})
	}

	for _, staleValue := range []string{"true", "false"} {
		t.Run("stale-"+staleValue, func(t *testing.T) {
			t.Setenv(envVar, staleValue)
			t.Setenv(mainEnv, "false")
			cfg := &config.Config{}
			store := newTestStore(t)
			if err := store.SetOverride(context.Background(), key, staleValue == "true"); err != nil {
				t.Fatalf("SetOverride retired key: %v", err)
			}

			values := ValuesFromConfig(cfg)
			if values[mainKey] {
				t.Fatalf("stale %s value enabled dynamic agents", staleValue)
			}
			if _, ok := OptionsFromConfig(cfg).EnvValues[envVar]; ok {
				t.Fatalf("retired environment variable %s is still read", envVar)
			}
			ApplyStatesToConfig(cfg, []RuntimeFlagState{{Key: key, EffectiveValue: true}})
			if cfg.Features.DynamicAgentRouting {
				t.Fatalf("retired override enabled dynamic agents for stale %s", staleValue)
			}

			states, err := NewService(store, OptionsFromConfig(cfg)).ListStates(context.Background())
			if err != nil {
				t.Fatalf("ListStates: %v", err)
			}
			for _, state := range states {
				if state.Key == key {
					t.Fatalf("retired flag %s appears in runtime catalog", key)
				}
			}
			overrides, err := store.ListOverrides(context.Background())
			if err != nil {
				t.Fatalf("ListOverrides: %v", err)
			}
			if got, ok := overrides[key]; !ok || got != (staleValue == "true") {
				t.Fatalf("saved override = %v, present=%v; want retained %s value", got, ok, staleValue)
			}

			encoded, err := json.Marshal(cfg.Features)
			if err != nil {
				t.Fatalf("marshal feature response: %v", err)
			}
			if strings.Contains(string(encoded), "dynamicCircuitBackoffEscalation") {
				t.Fatalf("retired feature remains in /api/v1/features shape: %s", encoded)
			}
		})
	}
}

func runtimeflagsRepoRoot(t *testing.T) string {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("locate test package: %v", err)
	}
	return filepath.Clean(filepath.Join(packageDir, "../../../.."))
}
