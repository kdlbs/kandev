package runtimeflags

import "github.com/kandev/kandev/internal/common/config"

// runtimeFlagRegistration keeps the public metadata and the typed config
// binding for a flag together. The function fields stay internal so the HTTP
// registry response remains metadata-only.
type runtimeFlagRegistration struct {
	definition RuntimeFlagDefinition
	read       func(*config.Config) bool
	apply      func(*config.Config, bool)
}

type runtimeFlagIdentity struct {
	key    string
	envVar string
}

// retiredRuntimeFlagIdentities is append-only. When a flag graduates, remove
// its active registration and move its identity here. Persisted overrides for
// unknown keys are intentionally retained, so neither the key nor environment
// variable may ever be reused for a different flag.
var retiredRuntimeFlagIdentities = []runtimeFlagIdentity{
	{key: "features.plugins", envVar: "KANDEV_FEATURES_PLUGINS"},
}

var registrations = []runtimeFlagRegistration{
	{
		definition: RuntimeFlagDefinition{
			Key:         "features.office",
			EnvVar:      "KANDEV_FEATURES_OFFICE",
			Kind:        KindFeature,
			Label:       "Office mode",
			Description: "Enables autonomous agent office workflows and related settings.",
			Stability:   StabilityExperimental,
			RiskLevel:   RiskMedium,
			RiskDescription: "Office mode is still evolving. Workflows, routes, and background automation " +
				"may change between releases and should be reviewed before relying on them.",
			RestartRequired: true,
			Mutable:         true,
		},
		read:  func(cfg *config.Config) bool { return cfg.Features.Office },
		apply: func(cfg *config.Config, value bool) { cfg.Features.Office = value },
	},
	{
		definition: RuntimeFlagDefinition{
			Key:         "features.appStatusBar",
			EnvVar:      "KANDEV_FEATURES_APP_STATUS_BAR",
			Kind:        KindFeature,
			Label:       "App status bar",
			Description: "Adds the global connection, optional host metrics, and plugin status surface.",
			Stability:   StabilityStable,
			RiskLevel:   RiskLow,
			RiskDescription: "Changing this adds or removes the desktop and tablet status bar and the phone Status drawer entry " +
				"after restart. It does not stop connections, metrics collection requested by other clients, or plugins.",
			RestartRequired: true,
			Mutable:         true,
		},
		read:  func(cfg *config.Config) bool { return cfg.Features.AppStatusBar },
		apply: func(cfg *config.Config, value bool) { cfg.Features.AppStatusBar = value },
	},
	{
		definition: RuntimeFlagDefinition{
			Key:         "features.auth",
			EnvVar:      "KANDEV_FEATURES_AUTH",
			Kind:        KindFeature,
			Label:       "Authentication & users",
			Description: "Requires every visitor to sign in and gives each user their own private workspaces. The first person to sign in after enabling becomes the admin.",
			Stability:   StabilityExperimental,
			RiskLevel:   RiskHigh,
			RiskDescription: "Turning this ON locks the instance behind a login after restart — the first visitor " +
				"completes a setup wizard and becomes the admin, and existing workspaces/secrets are assigned to " +
				"them. Turning it OFF after restart makes the instance open to anyone who can reach it again. " +
				"Enable it before exposing kandev on a shared or public network.",
			RestartRequired: true,
			Mutable:         true,
		},
		read:  func(cfg *config.Config) bool { return cfg.Features.Auth },
		apply: func(cfg *config.Config, value bool) { cfg.Features.Auth = value },
	},
	{
		definition: RuntimeFlagDefinition{
			Key:         "features.claudeBackgroundPromptHandoff",
			EnvVar:      "KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF",
			Kind:        KindFeature,
			Label:       "Claude background prompt handoff",
			Description: "Allows Claude Code to accept a new prompt after its foreground yields while recognized background work remains active.",
			Stability:   StabilityExperimental,
			RiskLevel:   RiskHigh,
			RiskDescription: "Claude ACP background lifecycle signals can be missing, delayed, duplicated, or ambiguous. " +
				"Enabling this experiment can misclassify session activity or dispatch overlapping prompts. " +
				"Use it only for controlled testing and disable it if a session behaves unexpectedly.",
			RestartRequired: true,
			Mutable:         true,
		},
		read:  func(cfg *config.Config) bool { return cfg.Features.ClaudeBackgroundPromptHandoff },
		apply: func(cfg *config.Config, value bool) { cfg.Features.ClaudeBackgroundPromptHandoff = value },
	},
	{
		definition: RuntimeFlagDefinition{
			Key:         "debug.devMode",
			EnvVar:      "KANDEV_DEBUG_DEV_MODE",
			Kind:        KindDebug,
			Label:       "Debug mode",
			Description: "Enables local diagnostic endpoints and agent message debug logs for troubleshooting backend, agent, and tool-call behavior.",
			Stability:   StabilityStable,
			RiskLevel:   RiskHigh,
			RiskDescription: "Debug mode can expose local diagnostic endpoints and write prompt, file, " +
				"and tool-call content to local debug logs. Enable it only on trusted machines.",
			RestartRequired: true,
			Mutable:         true,
			ImpliedEnvVars: []string{
				envDebugPprofEnabled,
				envDebugAgentMessages,
			},
		},
		read:  func(cfg *config.Config) bool { return cfg.Debug.DevMode || cfg.Debug.PprofEnabled },
		apply: applyDebugMode,
	},
}

func Definitions() []RuntimeFlagDefinition {
	out := make([]RuntimeFlagDefinition, len(registrations))
	for i, registration := range registrations {
		out[i] = publicDefinition(registration.definition)
	}
	return out
}

func DefinitionByKey(key string) (RuntimeFlagDefinition, bool) {
	registration, ok := registrationByKey(key)
	if !ok {
		return RuntimeFlagDefinition{}, false
	}
	return publicDefinition(registration.definition), true
}

func registrationByKey(key string) (runtimeFlagRegistration, bool) {
	for _, registration := range registrations {
		if registration.definition.Key == key {
			return registration, true
		}
	}
	return runtimeFlagRegistration{}, false
}

func publicDefinition(definition RuntimeFlagDefinition) RuntimeFlagDefinition {
	definition.ImpliedEnvVars = append([]string(nil), definition.ImpliedEnvVars...)
	return definition
}
