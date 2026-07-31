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
		Key:         featureAppStatusBarKey,
		EnvVar:      envFeaturesAppStatusBar,
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
	{
		Key:         featureAuthKey,
		EnvVar:      envFeaturesAuth,
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
	{
		Key:         featureClaudeBackgroundPromptHandoffKey,
		EnvVar:      envFeaturesClaudeBackgroundPromptHandoff,
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
