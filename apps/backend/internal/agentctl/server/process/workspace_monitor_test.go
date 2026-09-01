package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWorkspaceTracker_StopsWhenWorkDirDeleted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows refuses to unlink a directory while a process holds a handle inside it; the scenario this test exercises cannot occur on Windows")
	}
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

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.15
func TestGetUntrackedFilesID_ExcludesNodeModules(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	t.Cleanup(wt.Stop)
	ctx := context.Background()

	baseline, err := wt.getUntrackedFilesID(ctx)
	if err != nil {
		t.Fatalf("failed to get baseline untracked fingerprint: %v", err)
	}

	const dependencyPath = "node_modules/monitor-package/index.js"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repoDir, dependencyPath)), 0o755); err != nil {
		t.Fatalf("failed to create dependency directory: %v", err)
	}
	writeFile(t, repoDir, dependencyPath, "dependency version 1\n")
	created, err := wt.getUntrackedFilesID(ctx)
	if err != nil {
		t.Fatalf("failed to fingerprint created dependency: %v", err)
	}
	if created != baseline {
		t.Fatalf("fingerprint changed after creating dependency content: before %q, after %q", baseline, created)
	}

	writeFile(t, repoDir, dependencyPath, "dependency version 2\n")
	modified, err := wt.getUntrackedFilesID(ctx)
	if err != nil {
		t.Fatalf("failed to fingerprint modified dependency: %v", err)
	}
	if modified != baseline {
		t.Fatalf("fingerprint changed after modifying dependency content: before %q, after %q", baseline, modified)
	}

	if err := os.Remove(filepath.Join(repoDir, dependencyPath)); err != nil {
		t.Fatalf("failed to remove dependency file: %v", err)
	}
	removed, err := wt.getUntrackedFilesID(ctx)
	if err != nil {
		t.Fatalf("failed to fingerprint removed dependency: %v", err)
	}
	if removed != baseline {
		t.Fatalf("fingerprint changed after removing dependency content: before %q, after %q", baseline, removed)
	}

	const ordinaryPath = "monitor-source.ts"
	writeFile(t, repoDir, ordinaryPath, "export const monitored = true;\n")
	ordinary, err := wt.getUntrackedFilesID(ctx)
	if err != nil {
		t.Fatalf("failed to fingerprint ordinary untracked content: %v", err)
	}
	if ordinary == baseline {
		t.Fatal("fingerprint did not change after creating ordinary untracked content")
	}
}
