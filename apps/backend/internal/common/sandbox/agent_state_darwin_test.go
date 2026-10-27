//go:build darwin

package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func darwinStateScope() AgentStateScope {
	return AgentStateScope{OrgID: "org", WorkspaceID: "workspace", SessionID: "session", AgentProfileID: "profile", InstallationID: "provider", ProviderKey: "sandbox"}
}

func TestAgentStateDarwinHoldsExclusiveLock(t *testing.T) {
	base := t.TempDir()
	state, err := OpenAgentState(base, darwinStateScope())
	require.NoError(t, err)
	home := state.Directories().Home
	_, err = OpenAgentState(base, darwinStateScope())
	require.ErrorIs(t, err, ErrInvalidPolicy, "two attempts cannot own the same agent state")
	require.NoError(t, state.Close())
	state, err = OpenAgentState(base, darwinStateScope())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, state.Close()) })
	require.Equal(t, home, state.Directories().Home)
}
