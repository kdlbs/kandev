package runtimeflags

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
func TestInterruptionContinuationGraduatedFlagContract(t *testing.T) {
	const key = retiredProviderInterruptionContinuationKey
	const envVar = retiredProviderInterruptionContinuationEnvVar

	_, active := DefinitionByKey(key)
	require.False(t, active, "the graduated flag must not have an active registration")
	require.NotContains(t, ValuesFromConfig(&config.Config{}), key)

	retired := false
	for _, identity := range retiredRuntimeFlagIdentities {
		if identity.key == key && identity.envVar == envVar {
			retired = true
			break
		}
	}
	require.True(t, retired, "the old key and environment variable stay reserved")

	defaults, err := profiles.FeatureFlagDefaults()
	require.NoError(t, err)
	require.NotContains(t, defaults, "provider_interruption_continuation")

	t.Setenv(envVar, "false")
	dir := t.TempDir()
	err = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("features:\n  provider_interruption_continuation: false\n"), 0o600)
	require.NoError(t, err)
	cfg, err := config.LoadWithPath(dir)
	require.NoError(t, err)
	require.NotContains(t, OptionsFromConfig(cfg).EnvValues, envVar)

	store := newTestStore(t)
	require.NoError(t, store.SetOverride(context.Background(), key, false))
	service := NewService(store, OptionsFromConfig(cfg))
	states, err := service.ListStates(context.Background())
	require.NoError(t, err)
	for _, state := range states {
		require.NotEqual(t, key, state.Key, "a stale persisted override must not restore the retired flag")
	}
	overrides, err := store.ListOverrides(context.Background())
	require.NoError(t, err)
	require.Contains(t, overrides, key)
	require.False(t, overrides[key], "graduation keeps old rows inert without deleting them")

	ApplyStatesToConfig(cfg, states)
	features, err := json.Marshal(cfg.Features)
	require.NoError(t, err)
	require.NotContains(t, string(features), "providerInterruptionContinuation")
	startup, err := json.Marshal(cfg.ManagedAgentctlStartupConfig())
	require.NoError(t, err)
	require.NotContains(t, string(startup), "providerInterruptionContinuation")

	for _, relPath := range []string{"profiles.yaml", "apps/backend/internal/common/config/config.go", "apps/web/lib/state/slices/features/types.ts"} {
		content, err := os.ReadFile(filepath.Join(repoRoot(t), relPath))
		require.NoError(t, err)
		require.NotContains(t, string(content), envVar)
		require.NotContains(t, string(content), key)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	packageDir, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(packageDir, "../../../.."))
}
