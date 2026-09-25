package runtime_test

import (
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestManagedToolPolicyLifecycle(t *testing.T) {
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		InstanceKey: "delivery-lead", ConversationRevision: 4, ApprovalRevision: 2,
		ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentToolNames: []string{"read_task", "update_task"},
	}
	require.NoError(t, policy.Validate())
	require.True(t, policy.Allows("coordinator.v1", "read_task"))
	require.False(t, policy.Allows("observer.v1", "read_task"))
	require.False(t, policy.Allows("coordinator.v1", "unselected"))

	profile := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)
	restored := mcpprofile.Normalize(profile)
	require.Equal(t, mcpprofile.SurfaceManagedConversation, restored.Surface)
	require.NotNil(t, restored.ManagedToolPolicy)
	require.Equal(t, policy, *restored.ManagedToolPolicy)

	profile.ManagedToolPolicy.AgentToolNames[0] = "changed"
	require.Equal(t, "read_task", restored.ManagedToolPolicy.AgentToolNames[0], "profile normalization must isolate resumed launch policy")

	stale := policy
	stale.ConversationRevision = 0
	require.Error(t, stale.Validate(), "a policy without a current conversation revision must not enter launch or recovery")
	duplicate := policy
	duplicate.AgentToolNames = []string{"read_task", "read_task"}
	require.Error(t, duplicate.Validate(), "duplicate tools make policy identity ambiguous")
}
