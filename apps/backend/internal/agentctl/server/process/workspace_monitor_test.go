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
	// Default mode is slow (30s) — set fast so the test exercises real polling
	// cadence rather than sitting on a 30s timer.
	wt.SetPollMode(PollModeFast)

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

func TestWorkspaceTracker_MonitorExitsWhenNoGitRepo(t *testing.T) {
	isolateTestGitEnv(t)

	// Create a plain directory with no git repo — resolveGitIndexPath returns ""
	plainDir, err := os.MkdirTemp("", "test-no-git-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(plainDir) })

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(plainDir, log)
	// Use 500ms intervals so the 5-failure threshold takes ~2.5s.
	// The fix should make monitorLoop exit immediately (well under 500ms).
	wt.filePollInterval = 500 * time.Millisecond
	wt.gitPollInterval = 500 * time.Millisecond

	if wt.gitIndexPath != "" {
		t.Fatalf("expected empty gitIndexPath for non-git directory, got %q", wt.gitIndexPath)
	}

	wt.Start(context.Background())

	// monitorLoop should exit immediately without attempting git commands.
	// Without the fix, it takes ~2.5s (5 failures × 500ms poll interval).
	done := make(chan struct{})
	go func() {
		wt.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Both goroutines exited quickly — success
	case <-time.After(1 * time.Second):
		t.Fatal("workspace tracker did not stop promptly when started without a valid git repo")
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
	wt.SetPollMode(PollModeFast)

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
