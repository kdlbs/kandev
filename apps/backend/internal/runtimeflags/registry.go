package runtimeflags

var definitions = []RuntimeFlagDefinition{
	{
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
	{
		Key:         "features.plugins",
		EnvVar:      "KANDEV_FEATURES_PLUGINS",
		Kind:        KindFeature,
		Label:       "Plugins",
		Description: "Enables the extensible plugin system and related settings.",
		Stability:   StabilityExperimental,
		RiskLevel:   RiskMedium,
		RiskDescription: "Plugins are still evolving. Loaded plugin code runs with backend " +
			"privileges, and behavior may change between releases and should be reviewed " +
			"before relying on it.",
		RestartRequired: true,
		Mutable:         true,
	},
	{
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
			"KANDEV_DEBUG_PPROF_ENABLED",
			"KANDEV_DEBUG_AGENT_MESSAGES",
		},
	},
}

func Definitions() []RuntimeFlagDefinition {
	out := make([]RuntimeFlagDefinition, len(definitions))
	copy(out, definitions)
	return out
}

func DefinitionByKey(key string) (RuntimeFlagDefinition, bool) {
	for _, def := range definitions {
		if def.Key == key {
			return def, true
		}
	}
	return RuntimeFlagDefinition{}, false
}
