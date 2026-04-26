package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestWorkspaceTracker_FallsBackToRepoSubdir verifies the fix for the
// "changes panel empty on multi-repo task" issue. When the agent's workspace
// path is the multi-repo task root (a plain holder directory), the workspace
// tracker should fall back to the first git-repo child so it can still emit
// git status updates for at least one repo.
func TestWorkspaceTracker_FallsBackToRepoSubdir(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Build a synthetic task root that holds the test repo as a real
	// sub-directory, matching what env_preparer creates for multi-repo tasks
	// (worktrees placed under ~/.kandev/tasks/{TaskDirName}/{RepoName}/).
	// We rename rather than symlink because the production layout uses real
	// directories — and os.ReadDir's IsDir() returns false for symlinks.
	taskRoot := t.TempDir()
	repoSubdir := filepath.Join(taskRoot, "frontend")
	if err := os.Rename(repoDir, repoSubdir); err != nil {
		t.Fatalf("rename: %v", err)
	}

	wt := NewWorkspaceTracker(taskRoot, newTestLogger(t))

	// The tracker must have resolved its workDir to the repo subdir, otherwise
	// getGitStatus would fail with "not a git repository".
	if wt.workDir == taskRoot {
		t.Fatalf("expected workDir to fall back to %q, got the bare task root %q", repoSubdir, taskRoot)
	}

	status, err := wt.getGitStatus(context.Background())
	if err != nil {
		t.Fatalf("getGitStatus on resolved subdir: %v", err)
	}
	if status.Branch == "" {
		t.Errorf("expected non-empty branch from the fallback subdir; got status=%+v", status)
	}
}

// TestWorkspaceTracker_NoFallbackForRealRepo verifies the fallback only kicks
// in when the work dir itself is non-git. Single-repo tasks must keep their
// previous behavior verbatim.
func TestWorkspaceTracker_NoFallbackForRealRepo(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	if wt.workDir != repoDir {
		t.Errorf("real git repo workDir should not be rewritten; got %q want %q", wt.workDir, repoDir)
	}
}

// TestWorkspaceTracker_NoFallbackWhenNoGitChild verifies that a truly empty
// task root (no git children) doesn't try to fall back to anything — the
// tracker keeps the original workDir and just emits no git data.
func TestWorkspaceTracker_NoFallbackWhenNoGitChild(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "not-a-repo"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	wt := NewWorkspaceTracker(tmp, newTestLogger(t))
	if wt.workDir != tmp {
		t.Errorf("expected workDir unchanged when no git child exists; got %q", wt.workDir)
	}
}
