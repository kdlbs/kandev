package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// seedWorktree builds a directory standing in for a session's worktree, with
// one tracked file and one gitignored file, and returns the path plus a
// closure that fails the test if anything under it changed.
func seedWorktree(t *testing.T) (string, func()) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "worktree")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))

	files := map[string]string{
		filepath.Join(root, "src", "main.go"): "package main\n\nfunc main() {}\n",
		filepath.Join(root, ".env.local"):     "SECRET=value\n",
	}
	for path, contents := range files {
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	}

	return root, func() {
		t.Helper()
		info, err := os.Stat(root)
		require.NoErrorf(t, err, "the worktree directory is gone after the stop")
		require.True(t, info.IsDir(), "the worktree path is no longer a directory")
		for path, want := range files {
			got, err := os.ReadFile(path) //nolint:gosec // test-owned path under t.TempDir()
			require.NoErrorf(t, err, "worktree file %s is gone after the stop", path)
			require.Equalf(t, want, string(got), "worktree file %s was modified by the stop", path)
		}
	}
}

// TestBackendShutdownLeavesTheWorktreeUntouchedOnBothStopPaths pins
// AC-EXECUTORS-SURVIVAL-001.7 across the branch this capability introduced.
// A worktree holds the user's uncommitted work, so neither outcome of a
// backend shutdown may touch it: not the survivable detach that leaves the
// agent running, and not the terminating stop the same shutdown takes when
// the capability is off. Only an explicit task teardown removes a worktree,
// and a shutdown is not one.
func TestBackendShutdownLeavesTheWorktreeUntouchedOnBothStopPaths(t *testing.T) {
	for _, tc := range []struct {
		name             string
		survivalEnabled  bool
		wantStopInstance bool
	}{
		{name: "survivable detach", survivalEnabled: true, wantStopInstance: false},
		{name: "terminating stop", survivalEnabled: false, wantStopInstance: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr, mockExecutor := newDetachTestManager(t, tc.survivalEnabled)
			worktree, assertUnchanged := seedWorktree(t)

			execution := newDetachTestExecution("exec-worktree", "session-worktree")
			execution.WorkspacePath = worktree
			require.NoError(t, mgr.executionStore.Add(execution))

			require.NoError(t, mgr.StopAgentWithReason(t.Context(), execution.ID, StopReasonBackendShutdown, false))

			// Guards the premise: if the branch under test stopped being
			// reached, the worktree assertion below would pass vacuously.
			if tc.wantStopInstance {
				require.NotEmpty(t, mockExecutor.stopInstanceCalls, "expected the terminating path")
			} else {
				require.Empty(t, mockExecutor.stopInstanceCalls, "expected the detach path")
			}
			assertUnchanged()
		})
	}
}
