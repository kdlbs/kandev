package main

import (
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestSandboxV2FixtureKeepsOperationsAndEvidenceSeparate(t *testing.T) {
	provider := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, provider.Close()) })
	for _, key := range []string{fixtureSandboxAlpha, fixtureSandboxBeta} {
		t.Run(key, func(t *testing.T) {
			ctx := &pluginsdk.SandboxRequestContext{ProtocolMajor: 2, ProviderKey: key, OperationId: "op", AttemptId: "attempt", Generation: 1}
			policy := &pluginsdk.SandboxPolicySnapshot{Digest: "policy", PolicyV2: &pluginsdk.SandboxPolicyV2{SchemaVersion: 2, StaticGrants: &pluginsdk.SandboxPermissionsV2{}, ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}}
			described, err := provider.DescribeSandboxProvider(t.Context(), &pluginsdk.DescribeSandboxProviderRequest{Context: ctx})
			require.NoError(t, err)
			require.Equal(t, uint32(2), described.Context.SchemaVersion)
			require.Contains(t, described.Description.CapabilitiesV2.OperationKinds, "http_endpoint")
			compiled, err := provider.CompileSandboxPolicy(t.Context(), &pluginsdk.CompileSandboxPolicyRequest{Context: ctx, Policy: policy})
			require.NoError(t, err)
			require.Nil(t, compiled.Compiled.EffectiveGrants)
			require.Equal(t, uint32(2), compiled.Compiled.EffectivePolicyV2.SchemaVersion)
			observation, err := provider.NormalizeSandboxStatus(t.Context(), &pluginsdk.NormalizeSandboxStatusRequest{Context: ctx, EvidenceSchema: "fixture.runtime.v2"})
			require.NoError(t, err)
			require.Nil(t, observation.Evidence)
			require.NotEmpty(t, observation.ObservationsV2)
			approval := &pluginsdk.SandboxApprovalV2{RequestId: "request", Operation: &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Tcp{Tcp: &pluginsdk.SandboxTCPDestinationV2{Host: "api.example.test", Port: 443, Transport: "tcp"}}}, Actor: &pluginsdk.SandboxApprovalActorV2{Value: &pluginsdk.SandboxApprovalActorV2_RuntimeService{RuntimeService: &pluginsdk.SandboxServiceBindingV2{ChannelId: "channel", ServiceKey: "proxy", RouteId: "route"}}}, SupportedDecisions: []string{"deny", "allow_once"}}
			body, err := protojson.Marshal(approval)
			require.NoError(t, err)
			normalized, err := provider.NormalizeSandboxApproval(t.Context(), &pluginsdk.NormalizeSandboxApprovalRequest{Context: ctx, VerifiedEvent: body})
			require.NoError(t, err)
			require.Nil(t, normalized.Operation)
			require.Equal(t, "channel", normalized.ApprovalV2.Actor.GetRuntimeService().ChannelId)
		})
	}
}
