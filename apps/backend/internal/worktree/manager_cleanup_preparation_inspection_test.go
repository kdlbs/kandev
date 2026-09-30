package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
// @covers AC-TASKS-RUNTIME-CLEANUP-001.9
func TestCleanupPreparationInspectionAttribution(t *testing.T) {
	t.Run("missing source with healthy sibling", testPreparationMissingSource)
	t.Run("missing linked metadata", testPreparationMissingMetadata)
	t.Run("missing checkout and source", testPreparationMissingCheckoutAndSource)
	t.Run("verified missing branch", testPreparationVerifiedAbsence)
	t.Run("command failures", testPreparationCommandFailures)
}

func newPreparationInspectionWorktree(t *testing.T, mgr *Manager, repositoryID string) *Worktree {
	t.Helper()
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID: "task-inspection", SessionID: "session-inspection", TaskTitle: "Inspection",
		RepositoryID: repositoryID, RepositoryPath: initGitRepoWithRemote(t),
		BaseBranch: "main", IntegrationRef: "main", TaskDirName: "task-inspection", RepoName: repositoryID,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt.Path, "preserve.txt"), []byte(repositoryID), 0600))
	return wt
}

func testPreparationMissingSource(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-inspection", "session-inspection", models.TaskSessionStateCompleted)
	broken := newPreparationInspectionWorktree(t, mgr, "broken-repository")
	healthy := newPreparationInspectionWorktree(t, mgr, "healthy-repository")
	before, err := store.GetWorktreesByTaskID(context.Background(), broken.TaskID)
	require.NoError(t, err)
	require.Len(t, before, 2)
	healthyHead := runGit(t, healthy.Path, "rev-parse", "HEAD")
	_, err = mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{healthy, broken})
	require.NoError(t, err)
	dirty, err := mgr.InspectDirtyWorktrees(context.Background(), []*Worktree{healthy, broken})
	require.NoError(t, err)
	require.Len(t, dirty, 2)
	require.NoError(t, os.RemoveAll(broken.RepositoryPath))
	for _, inventory := range [][]*Worktree{{broken, healthy}, {healthy, broken}} {
		assertPreparationInspectionFailure(t, mgr, context.Background(), inventory,
			CleanupInspectionReasonRepoUnavailable, nil)
	}
	after, err := store.GetWorktreesByTaskID(context.Background(), broken.TaskID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, healthyHead, runGit(t, healthy.Path, "rev-parse", "HEAD"))
	for _, wt := range []*Worktree{healthy, broken} {
		content, err := os.ReadFile(filepath.Join(wt.Path, "preserve.txt"))
		require.NoError(t, err)
		require.Equal(t, wt.RepositoryID, string(content))
	}
}

func testPreparationMissingMetadata(t *testing.T) {
	mgr, _ := newReferenceCleanupTestManager(t)
	wt := newPreparationInspectionWorktree(t, mgr, "metadata-repository")
	inspection := inspectLinkedWorktree(wt.Path)
	require.Equal(t, linkedWorktreeHealthy, inspection.class)
	head := runGit(t, wt.RepositoryPath, "rev-parse", "refs/heads/"+wt.Branch)
	require.NoError(t, os.RemoveAll(inspection.adminPath))
	assertPreparationInspectionFailure(t, mgr, context.Background(), []*Worktree{wt},
		CleanupInspectionReasonRepoUnavailable, nil)
	require.Equal(t, head, runGit(t, wt.RepositoryPath, "rev-parse", "refs/heads/"+wt.Branch))
	content, err := os.ReadFile(filepath.Join(wt.Path, "preserve.txt"))
	require.NoError(t, err)
	require.Equal(t, wt.RepositoryID, string(content))
}

func testPreparationMissingCheckoutAndSource(t *testing.T) {
	mgr, _ := newReferenceCleanupTestManager(t)
	wt := newPreparationInspectionWorktree(t, mgr, "absent-repository")
	require.NoError(t, os.RemoveAll(wt.Path))
	require.NoError(t, os.RemoveAll(wt.RepositoryPath))
	_, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{wt})
	assertPreparationInspectionError(t, err, CleanupInspectionStageBranch,
		CleanupInspectionReasonRepoUnavailable, nil)
}

func testPreparationVerifiedAbsence(t *testing.T) {
	mgr, _ := newReferenceCleanupTestManager(t)
	wt := newPreparationInspectionWorktree(t, mgr, "absent-branch")
	runGit(t, wt.RepositoryPath, "worktree", "remove", "--force", wt.Path)
	runGit(t, wt.RepositoryPath, "branch", "-D", wt.Branch)
	oids, err := mgr.CaptureCleanupHeadOIDs(context.Background(), []*Worktree{wt})
	require.NoError(t, err)
	require.Empty(t, oids)
	dirty, err := mgr.InspectDirtyWorktrees(context.Background(), []*Worktree{wt})
	require.NoError(t, err)
	require.Empty(t, dirty)
}

func testPreparationCommandFailures(t *testing.T) {
	for _, name := range []string{"missing git", "exit 128", "cancelled", "deadline", "malformed pointer"} {
		t.Run(name, func(t *testing.T) {
			mgr, _ := newReferenceCleanupTestManager(t)
			wt := newPreparationInspectionWorktree(t, mgr, "command-repository")
			ctx, reason, cause := prepareInspectionFailure(t, name, wt)
			assertPreparationInspectionFailure(t, mgr, ctx, []*Worktree{wt}, reason, cause)
			content, err := os.ReadFile(filepath.Join(wt.Path, "preserve.txt"))
			require.NoError(t, err)
			require.Equal(t, wt.RepositoryID, string(content))
		})
	}
}

func prepareInspectionFailure(t *testing.T, name string, wt *Worktree) (context.Context, string, error) {
	t.Helper()
	ctx := context.Background()
	switch name {
	case "missing git":
		t.Setenv("PATH", t.TempDir())
		return ctx, CleanupInspectionReasonGitStartFailed, nil
	case "exit 128":
		if runtime.GOOS == "windows" {
			t.Skip("Git shim requires a POSIX shell")
		}
		shim := writeFakeGitScript(t, "printf 'private-git-output' >&2\nexit 128")
		t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))
	case "cancelled":
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		return cancelled, CleanupInspectionReasonContextCanceled, context.Canceled
	case "deadline":
		expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		return expired, CleanupInspectionReasonDeadlineExceeded, context.DeadlineExceeded
	case "malformed pointer":
		require.NoError(t, os.WriteFile(filepath.Join(wt.Path, ".git"), []byte("invalid pointer\n"), 0600))
	}
	return ctx, CleanupInspectionReasonCommandFailed, nil
}

func assertPreparationInspectionFailure(
	t *testing.T, mgr *Manager, ctx context.Context, inventory []*Worktree, reason string, cause error,
) {
	t.Helper()
	t.Run("identity", func(t *testing.T) {
		oids, err := mgr.CaptureCleanupHeadOIDs(ctx, inventory)
		assertPreparationInspectionError(t, err, CleanupInspectionStageCommit, reason, cause)
		require.Empty(t, oids)
	})
	t.Run("dirty state", func(t *testing.T) {
		dirty, err := mgr.InspectDirtyWorktrees(ctx, inventory)
		assertPreparationInspectionError(t, err, "working_tree_status", reason, cause)
		require.Empty(t, dirty)
	})
}

func assertPreparationInspectionError(t *testing.T, err error, stage, reason string, cause error) {
	t.Helper()
	var inspection *CleanupInspectionError
	require.ErrorAs(t, err, &inspection)
	require.Equal(t, stage, inspection.Stage)
	require.Equal(t, reason, inspection.Reason)
	if cause != nil {
		require.ErrorIs(t, err, cause)
	} else if reason == CleanupInspectionReasonCommandFailed {
		var exitError *exec.ExitError
		require.True(t, errors.As(err, &exitError), "original Git exit must survive wrapping: %v", err)
	}
	require.NotContains(t, err.Error(), "private-git-output")
}
