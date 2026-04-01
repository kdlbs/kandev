package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceTracker_StopsWhenWorkDirDeleted(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	wt.gitPollInterval = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wt.Start(ctx)

	// Delete the work directory to simulate worktree removal
	if err := os.RemoveAll(repoDir); err != nil {
		t.Fatalf("failed to remove workdir: %v", err)
	}

	// Both monitorLoop and pollGitChanges should exit within a few poll cycles
	done := make(chan struct{})
	go func() {
		wt.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Both goroutines exited — success
	case <-time.After(5 * time.Second):
		t.Fatal("workspace tracker goroutines did not stop after workdir was deleted")
	}
}

func TestWorkspaceTracker_StopsWhenGitBroken(t *testing.T) {
	isolateTestGitEnv(t)

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	// Use fast poll intervals so the test completes quickly
	wt.filePollInterval = 50 * time.Millisecond
	wt.gitPollInterval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wt.Start(ctx)

	// Corrupt the git repository by removing .git/HEAD.
	// The directory still exists, but git commands will fail with exit 128.
	headPath := filepath.Join(repoDir, ".git", "HEAD")
	if err := os.Remove(headPath); err != nil {
		t.Fatalf("failed to remove .git/HEAD: %v", err)
	}

	// Both loops should stop after maxConsecutiveGitFailures iterations
	done := make(chan struct{})
	go func() {
		wt.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Both goroutines exited — success
	case <-time.After(5 * time.Second):
		t.Fatal("workspace tracker goroutines did not stop after git was broken")
	}
}
