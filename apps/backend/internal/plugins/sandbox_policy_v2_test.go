package plugins

import (
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSandboxV2EndpointRejectsAmbiguousOrUnboundedTargets(t *testing.T) {
	endpoint := &pluginsdk.SandboxHTTPEndpointV2{RouteId: "api", Origin: "https://api.example.test", Method: "GET", Path: "/v1/items"}
	require.True(t, validSandboxEndpointV2(endpoint))
	for _, mutate := range []func(*pluginsdk.SandboxHTTPEndpointV2){
		func(v *pluginsdk.SandboxHTTPEndpointV2) { v.Origin = "https://api.example.test:0" },
		func(v *pluginsdk.SandboxHTTPEndpointV2) { v.Origin = "https://api.example.test:65536" },
		func(v *pluginsdk.SandboxHTTPEndpointV2) { v.Origin = "https://user:secret@api.example.test" },
		func(v *pluginsdk.SandboxHTTPEndpointV2) { v.Method = "CONNECT" },
		func(v *pluginsdk.SandboxHTTPEndpointV2) { v.Path = "/v1/%2fadmin" },
		func(v *pluginsdk.SandboxHTTPEndpointV2) { v.Path = "/v1/../admin" },
		func(v *pluginsdk.SandboxHTTPEndpointV2) { v.Query = "key=one&key=two" },
	} {
		value := proto.Clone(endpoint).(*pluginsdk.SandboxHTTPEndpointV2)
		mutate(value)
		require.False(t, validSandboxEndpointV2(value), "invalid endpoint: %s %s", value.Origin, value.Path)
	}
}

func TestSandboxV2PolicyRejectsMissingResourcesAndCycles(t *testing.T) {
	digest := strings.Repeat("a", 64)
	command := &pluginsdk.SandboxCommandInvocationV2{CommandId: "tool", ExecutablePath: "/usr/bin/tool", ExecutableSha256: digest, Caller: "session", WorkDirReference: "work_dir", ResourcePolicyDigest: digest}
	policy := &pluginsdk.SandboxPolicyV2{SchemaVersion: 2, StaticGrants: &pluginsdk.SandboxPermissionsV2{Commands: []*pluginsdk.SandboxCommandInvocationV2{command}}, ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}
	require.False(t, validSandboxPolicyV2(policy))
	resource := &pluginsdk.SandboxResourcePolicyV2{Digest: digest, Permissions: &pluginsdk.SandboxPermissionsV2{}}
	policy.ResourcePolicies = []*pluginsdk.SandboxResourcePolicyV2{resource}
	require.True(t, validSandboxPolicyV2(policy))
	resource.Permissions.Commands = []*pluginsdk.SandboxCommandInvocationV2{command}
	require.False(t, validSandboxPolicyV2(policy))
}

func TestSandboxV2CompilationCannotChangeTypedPermissions(t *testing.T) {
	request, compiled := sandboxCompilationFixture()
	request.Context = &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}
	request.Policy.Grants = nil
	request.Policy.PolicyV2 = &pluginsdk.SandboxPolicyV2{SchemaVersion: 2, StaticGrants: &pluginsdk.SandboxPermissionsV2{}, ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}
	compiled.EffectiveGrants = nil
	compiled.EffectivePolicyV2 = proto.Clone(request.Policy.PolicyV2).(*pluginsdk.SandboxPolicyV2)
	reply := &pluginsdk.CompileSandboxPolicyResponse{Compiled: compiled}
	require.NoError(t, validateSandboxReplyPayload(request, reply))
	compiled.EffectivePolicyV2.ApprovalEnvelope.KandevMcp = true
	require.Error(t, validateSandboxReplyPayload(request, reply))
}

func TestSandboxV2CanCarryLegacyPolicyWithoutRewritingItsMeaning(t *testing.T) {
	request, compiled := sandboxCompilationFixture()
	request.Context = &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}
	require.True(t, validSandboxRequestPayload(request))
	require.NoError(t, validateSandboxReplyPayload(request, &pluginsdk.CompileSandboxPolicyResponse{Compiled: compiled}))
	require.Nil(t, request.Policy.PolicyV2)
	compiled.EffectivePolicyV2 = &pluginsdk.SandboxPolicyV2{SchemaVersion: 2, StaticGrants: &pluginsdk.SandboxPermissionsV2{}, ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}
	require.Error(t, validateSandboxReplyPayload(request, &pluginsdk.CompileSandboxPolicyResponse{Compiled: compiled}))
}
