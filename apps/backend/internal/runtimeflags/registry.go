package runtimeflags

var definitions = []RuntimeFlagDefinition{
	{
		Key:         featureOfficeKey,
		EnvVar:      envFeaturesOffice,
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
		Key:         featurePluginsKey,
		EnvVar:      envFeaturesPlugins,
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
		Key:         featureAppStatusBarKey,
		EnvVar:      envFeaturesAppStatusBar,
		Kind:        KindFeature,
		Label:       "App status bar",
		Description: "Shows the global connection, optional system metrics, and plugin status surface.",
		Stability:   StabilityStable,
		RiskLevel:   RiskLow,
		RiskDescription: "Disabling this hides the desktop and tablet status bar and the phone Status drawer entry " +
			"after restart. It does not stop connections, metrics collection requested by other clients, or plugins.",
		RestartRequired: true,
		Mutable:         true,
	},
	{
		Key:         debugDevModeKey,
		EnvVar:      envDebugDevMode,
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
