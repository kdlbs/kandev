package runtimeflags

import (
	"os"
	"reflect"
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

func TestOptionsFromConfigIgnoresRetiredAppStatusBarEnv(t *testing.T) {
	preserveEnv(t, retiredAppStatusBarEnvVar)
	t.Setenv(retiredAppStatusBarEnvVar, "TRUE")

	opts := OptionsFromConfig(&config.Config{})

	if _, ok := opts.EnvValues[retiredAppStatusBarEnvVar]; ok {
		t.Fatal("retired KANDEV_FEATURES_APP_STATUS_BAR remains active")
	}
}

func TestOptionsFromConfigParsesClaudeBackgroundPromptHandoffEnv(t *testing.T) {
	preserveEnv(t, "KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF")
	t.Setenv("KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF", "TRUE")

	opts := OptionsFromConfig(&config.Config{})

	if !opts.EnvValues["KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF"] {
		t.Fatal("KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF TRUE parsed false, want true")
	}
}

func TestApplyStatesToConfigIgnoresRetiredAppStatusBar(t *testing.T) {
	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{
		Key:            retiredAppStatusBarKey,
		EffectiveValue: true,
	}})

	if _, ok := ValuesFromConfig(cfg)[retiredAppStatusBarKey]; ok {
		t.Fatal("ValuesFromConfig surfaced retired features.appStatusBar")
	}
}

func TestValuesFromConfigIncludesClaudeBackgroundPromptHandoff(t *testing.T) {
	cfg := &config.Config{}
	field := reflect.ValueOf(&cfg.Features).Elem().FieldByName("ClaudeBackgroundPromptHandoff")
	if !field.IsValid() {
		t.Fatal("FeaturesConfig.ClaudeBackgroundPromptHandoff field missing")
	}
	field.SetBool(true)

	values := ValuesFromConfig(cfg)

	if !values["features.claudeBackgroundPromptHandoff"] {
		t.Fatal("ValuesFromConfig did not surface features.claudeBackgroundPromptHandoff = true")
	}
}

func TestApplyStatesToConfigSetsClaudeBackgroundPromptHandoff(t *testing.T) {
	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{
		Key:            "features.claudeBackgroundPromptHandoff",
		EffectiveValue: true,
	}})

	field := reflect.ValueOf(&cfg.Features).Elem().FieldByName("ClaudeBackgroundPromptHandoff")
	if !field.IsValid() || !field.Bool() {
		t.Fatal("ApplyStatesToConfig did not set Features.ClaudeBackgroundPromptHandoff = true")
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

func TestOptionsFromConfigParsesClaudeMidTurnSteeringEnv(t *testing.T) {
	preserveEnv(t, "KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING")
	t.Setenv("KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING", "TRUE")

	opts := OptionsFromConfig(&config.Config{})

	if !opts.EnvValues["KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING"] {
		t.Fatal("KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING TRUE parsed false, want true")
	}
}

func TestValuesFromConfigIncludesClaudeMidTurnSteering(t *testing.T) {
	cfg := &config.Config{}
	field := reflect.ValueOf(&cfg.Features).Elem().FieldByName("ClaudeMidTurnSteering")
	if !field.IsValid() {
		t.Fatal("FeaturesConfig.ClaudeMidTurnSteering field missing")
	}
	field.SetBool(true)

	values := ValuesFromConfig(cfg)

	if !values["features.claudeMidTurnSteering"] {
		t.Fatal("ValuesFromConfig did not surface features.claudeMidTurnSteering = true")
	}
}

func TestApplyStatesToConfigSetsClaudeMidTurnSteering(t *testing.T) {
	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{
		Key:            "features.claudeMidTurnSteering",
		EffectiveValue: true,
	}})

	field := reflect.ValueOf(&cfg.Features).Elem().FieldByName("ClaudeMidTurnSteering")
	if !field.IsValid() || !field.Bool() {
		t.Fatal("ApplyStatesToConfig did not set Features.ClaudeMidTurnSteering = true")
	}
}

// TestClaudeMidTurnSteeringIsIndependentOfBackgroundHandoff pins that the two
// experiments are separately killable: enabling one must not enable the other.
func TestClaudeMidTurnSteeringIsIndependentOfBackgroundHandoff(t *testing.T) {
	cfg := &config.Config{}
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{
		Key:            "features.claudeMidTurnSteering",
		EffectiveValue: true,
	}})

	if !cfg.Features.ClaudeMidTurnSteering {
		t.Fatal("mid-turn steering did not become enabled")
	}
	if cfg.Features.ClaudeBackgroundPromptHandoff {
		t.Fatal("enabling mid-turn steering also enabled the background prompt handoff experiment")
	}
}
