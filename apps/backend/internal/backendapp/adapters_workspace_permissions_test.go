package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestWorkspacePermissionRequiresExactOneTimeOption(t *testing.T) {
	permissions := []streams.PendingAgentPermission{{SessionID: "session", RequestID: "generation", PendingID: "pending", Options: []streams.PermissionChoice{
		{OptionID: "yes", Kind: streams.PermissionOptionKindAllowOnce},
		{OptionID: "no", Kind: streams.PermissionOptionKindRejectOnce},
		{OptionID: "always", Kind: streams.PermissionOptionKindAllowAlways},
	}}}
	command := shared.WorkspaceTaskCommand{SessionID: "session", RequestID: "generation", PendingID: "pending", OptionID: "yes"}
	require.NoError(t, validateWorkspacePermissionChoice(permissions, command))
	command.OptionID = "no"
	require.NoError(t, validateWorkspacePermissionChoice(permissions, command))
	for _, option := range []string{"always", "invented", ""} {
		command.OptionID = option
		require.Error(t, validateWorkspacePermissionChoice(permissions, command))
	}
	command.OptionID, command.RequestID = "yes", "stale-generation"
	require.Error(t, validateWorkspacePermissionChoice(permissions, command))
	command.RequestID, command.SessionID = "generation", "foreign"
	require.Error(t, validateWorkspacePermissionChoice(permissions, command))
}
