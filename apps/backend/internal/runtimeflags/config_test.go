package runtimeflags

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

func TestApplyStatesToConfigClearsProfileDebugEnvWhenDisabled(t *testing.T) {
	for _, name := range []string{
		"KANDEV_DEBUG_DEV_MODE",
		"KANDEV_DEBUG_PPROF_ENABLED",
		"KANDEV_DEBUG_AGENT_MESSAGES",
	} {
		preserveEnv(t, name)
	}
	_ = os.Unsetenv("KANDEV_DEBUG_PPROF_ENABLED")
	_ = os.Unsetenv("KANDEV_DEBUG_AGENT_MESSAGES")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")

	if _, _, err := profiles.ApplyProfile(); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if os.Getenv("KANDEV_DEBUG_AGENT_MESSAGES") != "true" {
		t.Fatal("profile did not enable agent message debug logs")
	}

	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{
		Key:            "debug.devMode",
		EffectiveValue: false,
	}})

	if cfg.Debug.DevMode {
		t.Fatal("Debug.DevMode = true, want false")
	}
	if cfg.Debug.PprofEnabled {
		t.Fatal("Debug.PprofEnabled = true, want false")
	}
	if _, ok := os.LookupEnv("KANDEV_DEBUG_AGENT_MESSAGES"); ok {
		t.Fatal("KANDEV_DEBUG_AGENT_MESSAGES remained set after disabled override")
	}
	if _, ok := os.LookupEnv("KANDEV_DEBUG_PPROF_ENABLED"); ok {
		t.Fatal("KANDEV_DEBUG_PPROF_ENABLED remained set after disabled override")
	}
}

func TestOptionsFromConfigParsesUppercaseTruthyEnv(t *testing.T) {
	preserveEnv(t, "KANDEV_FEATURES_OFFICE")
	t.Setenv("KANDEV_FEATURES_OFFICE", "TRUE")

	opts := OptionsFromConfig(&config.Config{})

	if !opts.EnvValues["KANDEV_FEATURES_OFFICE"] {
		t.Fatal("KANDEV_FEATURES_OFFICE TRUE parsed false, want true")
	}
}

func TestOptionsFromConfigParsesUppercaseTruthyEnvForPlugins(t *testing.T) {
	preserveEnv(t, "KANDEV_FEATURES_PLUGINS")
	t.Setenv("KANDEV_FEATURES_PLUGINS", "TRUE")

	opts := OptionsFromConfig(&config.Config{})

	if !opts.EnvValues["KANDEV_FEATURES_PLUGINS"] {
		t.Fatal("KANDEV_FEATURES_PLUGINS TRUE parsed false, want true")
	}
}

func TestValuesFromConfigIncludesPlugins(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Plugins = true

	values := ValuesFromConfig(cfg)

	if !values["features.plugins"] {
		t.Fatal("ValuesFromConfig did not surface features.plugins = true")
	}
}

func TestApplyStatesToConfigSetsPlugins(t *testing.T) {
	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{
		Key:            "features.plugins",
		EffectiveValue: true,
	}})

	if !cfg.Features.Plugins {
		t.Fatal("ApplyStatesToConfig did not set Features.Plugins = true")
	}
}

func TestApplyStatesToConfigMarksImpliedDebugEnvAsApplied(t *testing.T) {
	for _, name := range []string{
		"KANDEV_DEBUG_PPROF_ENABLED",
		"KANDEV_DEBUG_AGENT_MESSAGES",
	} {
		preserveEnv(t, name)
	}

	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{
		Key:            "debug.devMode",
		EffectiveValue: true,
	}})
	opts := OptionsFromConfig(cfg)

	for _, name := range []string{
		"KANDEV_DEBUG_PPROF_ENABLED",
		"KANDEV_DEBUG_AGENT_MESSAGES",
	} {
		if !opts.EnvValues[name] {
			t.Fatalf("%s was not enabled", name)
		}
		if opts.IsExplicitEnv(name) {
			t.Fatalf("%s reported explicit, want profile-applied", name)
		}
	}
}

func preserveEnv(t *testing.T, name string) {
	t.Helper()
	value, ok := os.LookupEnv(name)
	_ = os.Unsetenv(name)
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(name, value)
			return
		}
		_ = os.Unsetenv(name)
	})
}
