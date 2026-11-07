package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-DELETE-RECOVERY-002.1
// @covers AC-TASKS-DELETE-RECOVERY-002.4
func TestDeleteUnavailableWorktreeIdentity(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-unknown", "session-unknown", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-unknown", "session-unknown")
	partial := *wt
	partial.RepositoryPath = ""
	_, enrichErr := mgr.InspectDirtyWorktrees(context.Background(), []*Worktree{&partial})
	require.NoError(t, enrichErr)
	marker := filepath.Join(filepath.Dir(wt.Path), ".kandev-workspace.json")
	original, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(marker, nil, 0600))
	_, err = mgr.InspectDirtyWorktrees(context.Background(), []*Worktree{wt})
	var inspection *CleanupInspectionError
	require.True(t, errors.As(err, &inspection), "marker failure must be classified, got %v", err)
	require.Error(t, mgr.CleanupWorktreesWithOptions(context.Background(), []*Worktree{wt}, WorktreeCleanupOptions{DiscardWorktreeChanges: true}))
	require.DirExists(t, wt.Path)
	require.NoError(t, os.WriteFile(marker, original, 0600))
	heads, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{wt})
	require.NoError(t, err)
	wt.CleanupHeadOID = heads[wt.ID]
	require.NoError(t, mgr.CleanupWorktreesWithOptions(context.Background(), []*Worktree{wt}, WorktreeCleanupOptions{DiscardWorktreeChanges: true}))
	require.NoDirExists(t, wt.Path)
}

func TestDeleteUnavailableMarkerPermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not restrict root")
	}
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-permission", "session-permission", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-permission", "session-permission")
	marker := filepath.Join(filepath.Dir(wt.Path), ".kandev-workspace.json")
	require.NoError(t, os.Chmod(marker, 0000))
	t.Cleanup(func() { _ = os.Chmod(marker, 0600) })
	_, err := mgr.InspectDirtyWorktrees(context.Background(), []*Worktree{wt})
	var inspection *CleanupInspectionError
	require.ErrorAs(t, err, &inspection)
	require.Equal(t, CleanupInspectionReasonRepoUnavailable, inspection.Reason)
	require.DirExists(t, wt.Path)
}
