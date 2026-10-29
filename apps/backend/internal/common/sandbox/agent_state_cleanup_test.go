//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveCapturedAgentStatePreservesSiblingAndIsIdempotent(t *testing.T) {
	base := t.TempDir()
	state, err := OpenAgentState(base, stateScope())
	require.NoError(t, err)
	snapshot := state.Snapshot()
	require.NoError(t, os.WriteFile(filepath.Join(snapshot.Home, "history"), []byte("owned"), 0600))
	require.NoError(t, state.Close())
	scope := stateScope()
	scope.SessionID = "sibling"
	sibling, err := OpenAgentState(base, scope)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sibling.Close()) })
	require.NoError(t, RemoveCapturedAgentState(base, snapshot))
	require.NoDirExists(t, filepath.Dir(snapshot.Home))
	require.DirExists(t, sibling.Directories().Home)
	require.NoError(t, RemoveCapturedAgentState(base, snapshot))
}

func TestRemoveCapturedAgentStateRejectsLiveLeaseAndForeignPath(t *testing.T) {
	base := t.TempDir()
	state, err := OpenAgentState(base, stateScope())
	require.NoError(t, err)
	snapshot := state.Snapshot()
	require.ErrorIs(t, RemoveCapturedAgentState(base, snapshot), ErrInvalidPolicy)
	require.DirExists(t, snapshot.Home)
	require.NoError(t, state.Close())
	foreign := *snapshot
	foreign.Home = t.TempDir()
	require.ErrorIs(t, RemoveCapturedAgentState(base, &foreign), ErrInvalidPolicy)
	require.DirExists(t, foreign.Home)
	require.DirExists(t, snapshot.Home)
}

func TestRemoveCapturedAgentStateRejectsSubstitutedOwnershipMarker(t *testing.T) {
	base := t.TempDir()
	state, err := OpenAgentState(base, stateScope())
	require.NoError(t, err)
	snapshot := state.Snapshot()
	require.NoError(t, state.Close())
	marker := filepath.Join(filepath.Dir(snapshot.Home), "owner.json")
	outside := filepath.Join(t.TempDir(), "marker")
	raw, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(outside, raw, 0600))
	require.NoError(t, os.Remove(marker))
	require.NoError(t, os.Symlink(outside, marker))
	require.ErrorIs(t, RemoveCapturedAgentState(base, snapshot), ErrInvalidPolicy)
	require.FileExists(t, outside)
	require.DirExists(t, snapshot.Home)
}
