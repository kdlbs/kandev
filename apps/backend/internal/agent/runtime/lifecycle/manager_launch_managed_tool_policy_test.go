package lifecycle

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestManagedToolPolicyProviderGate(t *testing.T) {
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
		ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentToolNames: []string{"read_task"},
	}
	profile := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)

	require.NoError(t, validateManagedToolPolicyProvider(nil, "codex", nil), "ordinary task launches have no managed-policy admission")
	ordinary := mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil)
	require.NoError(t, validateManagedToolPolicyProvider(&ordinary, "codex", nil), "ordinary task MCP profiles must pass through")

	err := validateManagedToolPolicyProvider(&profile, "codex", &agents.RuntimeConfig{})
	require.ErrorIs(t, err, ErrManagedToolPolicyUnsupported)

	err = validateManagedToolPolicyProvider(&profile, "codex", &agents.RuntimeConfig{SupportsManagedToolPolicy: true})
	require.NoError(t, err)

	profile.ManagedToolPolicy.ConversationRevision = 0
	err = validateManagedToolPolicyProvider(&profile, "codex", &agents.RuntimeConfig{SupportsManagedToolPolicy: true})
	require.ErrorIs(t, err, ErrManagedToolPolicyInvalid)
	require.False(t, errors.Is(err, ErrManagedToolPolicyUnsupported))
}

func TestBuildLaunchMetadataUsesValidatedManagedPolicy(t *testing.T) {
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
		ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentToolNames: []string{"read_task"},
	}
	profile := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)
	metadata := buildLaunchMetadata(&LaunchRequest{
		Metadata:   map[string]interface{}{mcpprofile.ManagedToolPolicyMetadataKey: `{"plugin_id":"attacker"}`},
		McpProfile: &profile,
	}, "", "", "")
	restored, err := mcpprofile.ParseManagedToolPolicyMetadata(metadata[mcpprofile.ManagedToolPolicyMetadataKey])
	require.NoError(t, err)
	require.Equal(t, policy, *restored)

	withoutPolicy := buildLaunchMetadata(&LaunchRequest{
		Metadata: map[string]interface{}{mcpprofile.ManagedToolPolicyMetadataKey: `{"plugin_id":"attacker"}`},
	}, "", "", "")
	require.NotContains(t, withoutPolicy, mcpprofile.ManagedToolPolicyMetadataKey)
}
