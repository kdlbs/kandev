package main

import (
	"context"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestSandboxProviderFixtureIsGenericAndCannotAttestConfinement(t *testing.T) {
	plugin := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
	provider, ok := any(plugin).(pluginsdk.SandboxProviderPlugin)
	require.True(t, ok, "fixture must implement the optional sandbox provider contract")
	for _, key := range []string{"fixture-alpha", "fixture-beta"} {
		t.Run(key, func(t *testing.T) {
			ctx := context.Background()
			identity := &pluginsdk.SandboxRequestContext{ProtocolMajor: 1, ProviderKey: key, OperationId: "operation-1", AttemptId: "attempt-1"}
			policy := &pluginsdk.SandboxPolicySnapshot{Digest: "policy-1", Grants: &pluginsdk.SandboxGrants{KandevMcp: true}}
			described, err := provider.DescribeSandboxProvider(ctx, &pluginsdk.DescribeSandboxProviderRequest{Context: identity})
			require.NoError(t, err)
			require.Equal(t, key, described.Description.DisplayName)
			require.NotEmpty(t, described.Description.Limitations)
			compiled, err := provider.CompileSandboxPolicy(ctx, &pluginsdk.CompileSandboxPolicyRequest{Context: identity, Policy: policy})
			require.NoError(t, err)
			require.True(t, compiled.Compiled.EffectiveGrants.KandevMcp)
			require.Equal(t, "sandbox.runtime", compiled.Compiled.Prefix[0].GetRuntimeReference())
			status, err := provider.NormalizeSandboxStatus(ctx, &pluginsdk.NormalizeSandboxStatusRequest{Context: identity, Policy: policy, Process: &pluginsdk.SandboxProcessIdentity{Pid: 42, BirthId: "birth-1"}})
			require.NoError(t, err)
			require.Empty(t, status.Evidence.PolicyDigest)
			require.Contains(t, status.Evidence.Limitations, "No confinement. Test fixture only.")
		})
	}
}
