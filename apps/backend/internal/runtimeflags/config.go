package runtimeflags

import (
	"os"
	"strings"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

const (
	envDebugPprofEnabled  = "KANDEV_DEBUG_PPROF_ENABLED"
	envDebugAgentMessages = "KANDEV_DEBUG_AGENT_MESSAGES"
)

func OptionsFromConfig(cfg *config.Config) Options {
	envValues := make(map[string]bool, len(registrations))
	for _, registration := range registrations {
		envValues[registration.definition.EnvVar] = isTruthy(os.Getenv(registration.definition.EnvVar))
		for _, impliedEnvVar := range registration.definition.ImpliedEnvVars {
			envValues[impliedEnvVar] = isTruthy(os.Getenv(impliedEnvVar))
		}
	}

	return Options{
		DefaultValues: ValuesFromConfig(cfg),
		RuntimeValues: ValuesFromConfig(cfg),
		EnvValues:     envValues,
		IsExplicitEnv: func(name string) bool {
			_, ok := os.LookupEnv(name)
			return ok && !profiles.WasApplied(name)
		},
	}
}

func ValuesFromConfig(cfg *config.Config) map[string]bool {
	values := make(map[string]bool, len(registrations))
	for _, registration := range registrations {
		values[registration.definition.Key] = registration.read(cfg)
	}
	return values
}

func ApplyStatesToConfig(cfg *config.Config, states []RuntimeFlagState) {
	for _, state := range states {
		registration, ok := registrationByKey(state.Key)
		if !ok {
			continue
		}
		registration.apply(cfg, state.EffectiveValue)
	}
}

func applyDebugMode(cfg *config.Config, enabled bool) {
	cfg.Debug.DevMode = enabled
	cfg.Debug.PprofEnabled = enabled
	if enabled {
		setIfNotExplicit(envDebugAgentMessages, "true")
		setIfNotExplicit(envDebugPprofEnabled, "true")
		return
	}
	unsetIfNotExplicit(envDebugAgentMessages)
	unsetIfNotExplicit(envDebugPprofEnabled)
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
