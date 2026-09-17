package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/stretchr/testify/require"
)

func TestAssistantFeatureGateRegisteredAndDefaultOff(t *testing.T) {
	def, ok := DefinitionByKey("features.personalAssistant")
	require.True(t, ok, "assistant needs an independent rollout toggle")
	require.Equal(t, "KANDEV_FEATURES_PERSONAL_ASSISTANT", def.EnvVar)
	require.True(t, def.RestartRequired)
	require.True(t, def.Mutable)
	cfg := &config.Config{}
	values := ValuesFromConfig(cfg)
	require.Contains(t, values, def.Key)
	require.False(t, values[def.Key])
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{Key: def.Key, EffectiveValue: true}})
	require.True(t, ValuesFromConfig(cfg)[def.Key])
	require.False(t, ValuesFromConfig(cfg)["features.orchestration"])
}
