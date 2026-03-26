package process

import (
	"context"
	"os"
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
