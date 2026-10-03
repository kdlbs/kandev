package runtimeflags

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
func TestContinuationConfigFlagContract(t *testing.T) {
	const key = "features.providerInterruptionContinuation"
	def, ok := DefinitionByKey(key)
	require.True(t, ok, "continuation registration must exist")
	require.Equal(t, "KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION", def.EnvVar)
	require.Equal(t, StabilityExperimental, def.Stability)
	require.Equal(t, RiskHigh, def.RiskLevel)
	require.True(t, def.RestartRequired)

	defaults, err := profiles.FeatureFlagDefaults()
	require.NoError(t, err)
	require.Contains(t, defaults, "provider_interruption_continuation")
	require.Equal(t, "false", defaults["provider_interruption_continuation"])
	cfg := &config.Config{}
	require.False(t, ValuesFromConfig(cfg)[key])
	ApplyStatesToConfig(cfg, []RuntimeFlagState{{Key: key, EffectiveValue: true}})
	require.True(t, ValuesFromConfig(cfg)[key])
	raw, err := json.Marshal(cfg.ManagedAgentctlStartupConfig())
	require.NoError(t, err)
	require.Contains(t, string(raw), `"providerInterruptionContinuation":true`)
}
