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
		name := name
		_ = os.Unsetenv(name)
		t.Cleanup(func() { _ = os.Unsetenv(name) })
	}
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
