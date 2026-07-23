package runtimeflags

import (
	"os"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

const (
	featureOfficeKey        = "features.office"
	featurePluginsKey       = "features.plugins"
	featureAppStatusBarKey  = "features.appStatusBar"
	debugDevModeKey         = "debug.devMode"
	envFeaturesOffice       = "KANDEV_FEATURES_OFFICE"
	envFeaturesPlugins      = "KANDEV_FEATURES_PLUGINS"
	envFeaturesAppStatusBar = "KANDEV_FEATURES_APP_STATUS_BAR"
	envDebugDevMode         = "KANDEV_DEBUG_DEV_MODE"
	envDebugPprofEnabled    = "KANDEV_DEBUG_PPROF_ENABLED"
	envDebugAgentMessages   = "KANDEV_DEBUG_AGENT_MESSAGES"
)

func OptionsFromConfig(cfg *config.Config) Options {
	return Options{
		DefaultValues: ValuesFromConfig(cfg),
		RuntimeValues: ValuesFromConfig(cfg),
		EnvValues: map[string]bool{
			envFeaturesOffice:       isTruthy(os.Getenv(envFeaturesOffice)),
			envFeaturesPlugins:      isTruthy(os.Getenv(envFeaturesPlugins)),
			envFeaturesAppStatusBar: isTruthy(os.Getenv(envFeaturesAppStatusBar)),
			envDebugDevMode:         isTruthy(os.Getenv(envDebugDevMode)),
			envDebugPprofEnabled:    isTruthy(os.Getenv(envDebugPprofEnabled)),
			envDebugAgentMessages:   isTruthy(os.Getenv(envDebugAgentMessages)),
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
		featureOfficeKey:       cfg.Features.Office,
		featurePluginsKey:      cfg.Features.Plugins,
		featureAppStatusBarKey: cfg.Features.AppStatusBar,
		debugDevModeKey:        debugEnabled,
	}
}

func ApplyStatesToConfig(cfg *config.Config, states []RuntimeFlagState) {
	for _, state := range states {
		switch state.Key {
		case featureOfficeKey:
			cfg.Features.Office = state.EffectiveValue
		case featurePluginsKey:
			cfg.Features.Plugins = state.EffectiveValue
		case featureAppStatusBarKey:
			cfg.Features.AppStatusBar = state.EffectiveValue
		case debugDevModeKey:
			cfg.Debug.DevMode = state.EffectiveValue
			cfg.Debug.PprofEnabled = state.EffectiveValue
			if state.EffectiveValue {
				setIfNotExplicit(envDebugAgentMessages, "true")
				setIfNotExplicit(envDebugPprofEnabled, "true")
			} else {
				unsetIfNotExplicit(envDebugAgentMessages)
				unsetIfNotExplicit(envDebugPprofEnabled)
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
