package sandboxruntime

import (
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestTypedPolicyPreparesEnvelopeAgentState(t *testing.T) {
	prepared := &Prepared{manager: &Manager{base: t.TempDir()}, binding: plugins.SandboxProviderBinding{InstallationID: "installation", ProviderKey: "provider"}}
	t.Cleanup(func() { require.NoError(t, prepared.releaseAgentState()) })
	resolved := &policy.Resolved{Revision: &policy.Revision{SchemaVersion: 2, Policy: policy.Policy{PolicyV2: &contract.PolicyV2{
		SchemaVersion:    2,
		StaticGrants:     contract.PermissionsV2{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir}}},
		ApprovalEnvelope: contract.PermissionsV2{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceAgentState, Writable: true}}},
	}}}}
	request := PrepareRequest{Selection: policy.SelectionContext{OrgID: "org", AgentProfileID: "agent"}, ExecutionID: "execution", WorkDir: t.TempDir()}
	err := prepared.prepareAgentState(request, resolved)
	require.NoError(t, err)
	require.NotNil(t, prepared.agentState, "conditional state references need an owned directory without granting initial access")
	static, typed, err := prepared.resolveRuntimePolicy(request, resolved)
	require.NoError(t, err)
	require.Equal(t, []contract.FilesystemGrant{{Path: request.WorkDir}}, static.Filesystem)
	require.Equal(t, []contract.FilesystemGrant{{Path: prepared.agentState.Directories().Home, Writable: true}}, typed.ApprovalEnvelope.Filesystem)
	require.Equal(t, contract.ReferenceAgentState, resolved.Revision.Policy.PolicyV2.ApprovalEnvelope.Filesystem[0].Reference,
		"binding attempt paths must not mutate the saved revision")
	request.WorkDir = prepared.manager.base
	_, _, err = prepared.resolveRuntimePolicy(request, resolved)
	require.ErrorIs(t, err, contract.ErrInvalidPolicy)
}

func TestWirePolicyDoesNotDropTypedPermissions(t *testing.T) {
	work := t.TempDir()
	typed := &contract.PolicyV2{SchemaVersion: 2,
		StaticGrants:     contract.PermissionsV2{Filesystem: []contract.FilesystemGrant{{Path: work}}, EnvironmentKeys: []string{"LANG"}, KandevMCP: true},
		ApprovalEnvelope: contract.PermissionsV2{TCP: []contract.TCPDestination{{Host: "api.example.test", Port: 443, Transport: contract.ProtocolTCP}}}}
	resolved := &policy.Resolved{Revision: &policy.Revision{ID: "revision", Number: 2, SchemaVersion: 2, Digest: "policy-digest",
		Policy: policy.Policy{ID: "policy", PolicyV2: &contract.PolicyV2{SchemaVersion: 2,
			StaticGrants: contract.PermissionsV2{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir}}}}}},
		Ceiling: &policy.Revision{Digest: "ceiling-digest"}}
	static, err := typed.StaticAccess()
	require.NoError(t, err)
	wire := wirePolicy(resolved, static, typed)
	require.Nil(t, wire.Grants, "typed policies must not travel through the legacy grant field")
	require.NotNil(t, wire.PolicyV2)
	require.Equal(t, uint32(2), wire.PolicyV2.SchemaVersion)
	require.Equal(t, work, wire.PolicyV2.StaticGrants.Filesystem[0].Path)
	require.True(t, wire.PolicyV2.StaticGrants.KandevMcp)
	require.Equal(t, []string{"LANG"}, wire.PolicyV2.StaticGrants.EnvironmentKeys)
	require.Empty(t, wire.PolicyV2.StaticGrants.Tcp)
	require.Equal(t, []*pluginsdk.SandboxTCPDestinationV2{{Host: "api.example.test", Port: 443, Transport: contract.ProtocolTCP}}, wire.PolicyV2.ApprovalEnvelope.Tcp)
}
