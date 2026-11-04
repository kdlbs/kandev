package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinAttachmentManifestComesFromExecutionOwner(t *testing.T) {
	for _, bound := range []bool{false, true} {
		manager, eventBus := createTestManagerWithTracking()
		execution := createTestExecution("execution", "task-a", "session")
		execution.WorkspaceID = "workspace"
		profile := builtinDispatchProfile(p.PackageCore)
		if bound {
			encoded, err := p.MarshalBuiltinBinding(profile)
			require.NoError(t, err)
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
		}
		forged := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
		event := agentctl.AgentEvent{Type: streams.EventTypeMCPAttachment, MCPAttachmentAttempt: &streams.MCPAttachmentAttempt{AttemptID: "attempt", BuiltinPackages: forged.BuiltinPackages}}
		require.True(t, manager.handleMCPAttachmentEvent(execution, &event, ""))
		events := eventBus.getStreamEvents()
		require.Len(t, events, 1)
		actual := events[0].Data.MCPAttachmentAttempt.BuiltinPackages
		if bound {
			require.Equal(t, profile.BuiltinPackages, actual)
		} else {
			require.Nil(t, actual)
		}
	}
}
