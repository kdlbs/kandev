package runtimeflags

import (
	"os"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

func OptionsFromConfig(cfg *config.Config) Options {
	return Options{
		DefaultValues: ValuesFromConfig(cfg),
		RuntimeValues: ValuesFromConfig(cfg),
		EnvValues: map[string]bool{
			"KANDEV_FEATURES_OFFICE":      isTruthy(os.Getenv("KANDEV_FEATURES_OFFICE")),
			"KANDEV_FEATURES_PLUGINS":     isTruthy(os.Getenv("KANDEV_FEATURES_PLUGINS")),
			"KANDEV_DEBUG_DEV_MODE":       isTruthy(os.Getenv("KANDEV_DEBUG_DEV_MODE")),
			"KANDEV_DEBUG_PPROF_ENABLED":  isTruthy(os.Getenv("KANDEV_DEBUG_PPROF_ENABLED")),
			"KANDEV_DEBUG_AGENT_MESSAGES": isTruthy(os.Getenv("KANDEV_DEBUG_AGENT_MESSAGES")),
		},
		IsExplicitEnv: func(name string) bool {
			_, ok := os.LookupEnv(name)
			return ok && !profiles.WasApplied(name)
		},
	}
}

func ValuesFromConfig(cfg *config.Config) map[string]bool {
	debugEnabled := cfg.Debug.DevMode || cfg.Debug.PprofEnabled
	return map[string]bool{
		"features.office":  cfg.Features.Office,
		"features.plugins": cfg.Features.Plugins,
		"debug.devMode":    debugEnabled,
	}
}

func ApplyStatesToConfig(cfg *config.Config, states []RuntimeFlagState) {
	for _, state := range states {
		switch state.Key {
		case "features.office":
			cfg.Features.Office = state.EffectiveValue
		case "features.plugins":
			cfg.Features.Plugins = state.EffectiveValue
		case "debug.devMode":
			cfg.Debug.DevMode = state.EffectiveValue
			cfg.Debug.PprofEnabled = state.EffectiveValue
			if state.EffectiveValue {
				setIfNotExplicit("KANDEV_DEBUG_AGENT_MESSAGES", "true")
				setIfNotExplicit("KANDEV_DEBUG_PPROF_ENABLED", "true")
			} else {
				unsetIfNotExplicit("KANDEV_DEBUG_AGENT_MESSAGES")
				unsetIfNotExplicit("KANDEV_DEBUG_PPROF_ENABLED")
			}
		}
	}
}

func RuntimeOptionsFromAppliedConfig(defaults map[string]bool, cfg *config.Config) Options {
	opts := OptionsFromConfig(cfg)
	opts.DefaultValues = defaults
	opts.RuntimeValues = ValuesFromConfig(cfg)
	return opts
}

func setIfNotExplicit(name, value string) {
	if _, ok := os.LookupEnv(name); ok && !profiles.WasApplied(name) {
		return
	}
	_ = os.Setenv(name, value)
	profiles.MarkApplied(name)
}

func unsetIfNotExplicit(name string) {
	if _, ok := os.LookupEnv(name); ok && !profiles.WasApplied(name) {
		return
	}
	_ = os.Unsetenv(name)
}

func isTruthy(s string) bool {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
