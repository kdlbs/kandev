package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadEffectiveSessionRuntimeConfigMergesInPrecedenceOrder(t *testing.T) {
	session := &TaskSession{
		AgentProfileSnapshot: map[string]interface{}{
			"agent_id": "provider-agent",
			"model":    "profile-model",
			"mode":     "profile-mode",
			"config_options": map[string]string{
				"profile_only":      "profile",
				"provider_replaced": "profile-value",
			},
		},
		Metadata: map[string]interface{}{
			SessionMetaKeyRuntimeConfig: SessionRuntimeConfig{
				Model: "provider-model",
				Mode:  "provider-mode",
				ConfigOptions: map[string]string{
					"model":             "provider-model",
					"mode":              "provider-mode",
					"agent":             "provider-agent",
					"provider_replaced": "provider-value",
					"provider_only":     "provider",
				},
			},
			SessionMetaKeySessionMode: "session-mode",
			SessionMetaKeyRuntimeConfigOverrides: SessionRuntimeConfig{
				Model: "override-model",
				Mode:  "override-mode",
				ConfigOptions: map[string]string{
					"model":             "override-model",
					"mode":              "override-mode",
					"provider_replaced": "override-value",
					"override_only":     "override",
				},
			},
		},
	}

	got, ok := LoadEffectiveSessionRuntimeConfig(session)
	require.True(t, ok)
	require.Equal(t, "override-model", got.Model)
	require.Equal(t, "override-mode", got.Mode)
	require.Equal(t, map[string]string{
		"provider_replaced": "override-value",
		"provider_only":     "provider",
		"override_only":     "override",
	}, got.ConfigOptions)
}

func TestLoadEffectiveSessionRuntimeConfigPreservesProviderAgentOption(t *testing.T) {
	session := &TaskSession{
		AgentProfileSnapshot: map[string]interface{}{
			"agent_id": "claude",
			"config_options": map[string]string{
				"agent": "build",
			},
		},
		Metadata: map[string]interface{}{
			SessionMetaKeyRuntimeConfig: SessionRuntimeConfig{
				ConfigOptions: map[string]string{"agent": "build"},
			},
		},
	}

	got, ok := LoadEffectiveSessionRuntimeConfig(session)
	require.True(t, ok)
	require.Equal(t, map[string]string{"agent": "build"}, got.ConfigOptions)
}

func TestLoadEffectiveSessionRuntimeConfigReturnsIndependentOptions(t *testing.T) {
	profileOptions := map[string]string{"profile": "value"}
	providerOptions := map[string]string{"provider": "value"}
	overrideOptions := map[string]string{"override": "value"}
	session := &TaskSession{
		AgentProfileSnapshot: map[string]interface{}{"config_options": profileOptions},
		Metadata: map[string]interface{}{
			SessionMetaKeyRuntimeConfig:          SessionRuntimeConfig{ConfigOptions: providerOptions},
			SessionMetaKeyRuntimeConfigOverrides: SessionRuntimeConfig{ConfigOptions: overrideOptions},
		},
	}

	got, ok := LoadEffectiveSessionRuntimeConfig(session)
	require.True(t, ok)
	got.ConfigOptions["profile"] = "changed"
	got.ConfigOptions["provider"] = "changed"
	got.ConfigOptions["override"] = "changed"

	require.Equal(t, "value", profileOptions["profile"])
	require.Equal(t, "value", providerOptions["provider"])
	require.Equal(t, "value", overrideOptions["override"])
}

func TestLoadEffectiveSessionRuntimeConfigAcceptsJSONRehydratedMetadata(t *testing.T) {
	got, ok := LoadEffectiveSessionRuntimeConfig(&TaskSession{
		AgentProfileSnapshot: map[string]interface{}{
			"model": "profile-model",
			"configOptions": map[string]interface{}{
				"effort":       "medium",
				"profile_only": "stale",
			},
		},
		Metadata: map[string]interface{}{
			SessionMetaKeyRuntimeConfig: map[string]interface{}{
				"model":          "provider-model",
				"config_options": map[string]interface{}{"effort": "high"},
			},
			SessionMetaKeySessionMode: "acceptEdits",
			SessionMetaKeyRuntimeConfigOverrides: map[string]interface{}{
				"config_options": map[string]interface{}{"effort": "low"},
			},
		},
	})

	require.True(t, ok)
	require.Equal(t, "provider-model", got.Model)
	require.Equal(t, "acceptEdits", got.Mode)
	require.Equal(t, map[string]string{"effort": "low"}, got.ConfigOptions)
}
