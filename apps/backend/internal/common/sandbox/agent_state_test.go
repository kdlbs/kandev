//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func stateScope() AgentStateScope {
	return AgentStateScope{OrgID: "org", WorkspaceID: "workspace", SessionID: "session", AgentProfileID: "profile", InstallationID: "provider", ProviderKey: "sandbox"}
}

func TestAgentStatePersistsAcrossAttemptsWithoutExposingSupervisorFiles(t *testing.T) {
	base := t.TempDir()
	state, err := OpenAgentState(base, stateScope())
	require.NoError(t, err)
	home := state.Directories().Home
	file := filepath.Join(home, "conversation")
	require.NoError(t, os.WriteFile(file, []byte("retained session"), 0600))
	intent := Grants{Filesystem: []FilesystemGrant{{Reference: "agent_state", Writable: true}}}
	grants, err := ResolveRuntimeDirectories(intent, t.TempDir(), state, []string{base})
	require.NoError(t, err)
	require.Equal(t, []FilesystemGrant{{Path: home, Writable: true}}, grants.Filesystem)
	_, err = ResolveRuntimeDirectories(Grants{Filesystem: []FilesystemGrant{{Path: home, Writable: true}}}, t.TempDir(), state, []string{base})
	require.ErrorIs(t, err, ErrInvalidPolicy, "a literal path cannot bypass protected storage")
	_, err = OpenAgentState(base, stateScope())
	require.ErrorIs(t, err, ErrInvalidPolicy, "two attempts cannot own the same agent state")
	require.NoError(t, state.Close())
	state, err = OpenAgentState(base, stateScope())
	require.NoError(t, err)
	defer func() { require.NoError(t, state.Close()) }()
	require.Equal(t, home, state.Directories().Home)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "retained session", string(content))
	scope := stateScope()
	scope.SessionID = "other-session"
	other, err := OpenAgentState(base, scope)
	require.NoError(t, err)
	defer func() { require.NoError(t, other.Close()) }()
	require.NotEqual(t, home, other.Directories().Home)
	require.ErrorIs(t, WithinCeiling(intent, Grants{Filesystem: []FilesystemGrant{{Reference: ReferenceWorkDir, Writable: true}}}), ErrOutsideCeiling)
	require.NoError(t, WithinCeiling(intent, intent))
	_, err = ResolveRuntimeDirectories(intent, t.TempDir(), nil, nil)
	require.ErrorIs(t, err, ErrInvalidPolicy)
}

func TestAgentStateRejectsReplacedChildDirectoryAndClosedLease(t *testing.T) {
	base := t.TempDir()
	state, err := OpenAgentState(base, stateScope())
	require.NoError(t, err)
	home := state.Directories().Home
	require.NoError(t, os.RemoveAll(home))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, home))
	intent := Grants{Filesystem: []FilesystemGrant{{Reference: "agent_state", Writable: true}}}
	_, err = ResolveRuntimeDirectories(intent, t.TempDir(), state, []string{base})
	require.ErrorIs(t, err, ErrInvalidPolicy)
	require.NoError(t, state.Close())
	_, err = OpenAgentState(base, stateScope())
	require.ErrorIs(t, err, ErrInvalidPolicy)
	require.DirExists(t, outside)
	clean, err := OpenAgentState(t.TempDir(), stateScope())
	require.NoError(t, err)
	require.NoError(t, clean.Close())
	_, err = ResolveRuntimeDirectories(intent, t.TempDir(), clean, nil)
	require.ErrorIs(t, err, ErrInvalidPolicy)
}

func TestUtilityAgentStateHasExecutionScopeAndOwnedCleanup(t *testing.T) {
	base := t.TempDir()
	scope := stateScope()
	scope.WorkspaceID = ""
	scope.SessionID = ""
	scope.ExecutionID = "utility-execution"
	state, err := OpenAgentState(base, scope)
	require.NoError(t, err)
	home := state.Directories().Home
	require.NoError(t, os.WriteFile(filepath.Join(home, "temporary"), []byte("utility"), 0600))
	require.NoError(t, state.Remove())
	require.NoDirExists(t, filepath.Dir(home))
	require.NoError(t, state.Close())
	other := scope
	other.SessionID = "session"
	_, err = OpenAgentState(base, other)
	require.ErrorIs(t, err, ErrInvalidPolicy, "state cannot have two lifetime owners")
}
