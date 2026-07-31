package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// TestRescanTrackerInheritsFocusedPollMode covers the regression flagged in
// review on #2113: a rescan adds a repository to a workspace that is already
// focused. No session mode transition happens, so the gateway pushes nothing,
// and without inheritance the new tracker would sit at its construction
// default and be demoted to slow by the grace timer while the user is
// actively looking at it.
func TestRescanTrackerInheritsFocusedPollMode(t *testing.T) {
	isolateTestGitEnv(t)
	taskRoot := t.TempDir()
	for _, name := range []string{"frontend", "backend"} {
		repoDir, cleanup := setupTestRepo(t)
		t.Cleanup(cleanup)
		if err := os.Rename(repoDir, filepath.Join(taskRoot, name)); err != nil {
			t.Fatalf("place %s: %v", name, err)
		}
	}

	mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
	t.Cleanup(func() {
		mgr.workspaceTracker.Stop()
		for _, tr := range mgr.repoTrackers {
			tr.Stop()
		}
	})

	// The user focuses the task: the gateway pushes fast once.
	mgr.SetWorkspacePollMode(context.Background(), PollModeFast)

	// A repository appears afterwards. The gateway stays silent because the
	// session's own mode did not change.
	newRepo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	if err := os.Rename(newRepo, filepath.Join(taskRoot, "worker")); err != nil {
		t.Fatalf("place worker: %v", err)
	}
	tracker := mgr.newTrackerForRepo(filepath.Join(taskRoot, "worker"), "worker")
	tracker.pollModeGrace = 50 * time.Millisecond
	tracker.Start(context.Background())
	t.Cleanup(tracker.Stop)

	if got := tracker.GetPollMode(); got != PollModeFast {
		t.Fatalf("tracker created during a focused session = %v, want %v", got, PollModeFast)
	}

	// Well past the grace deadline it must still be fast — inheriting the push
	// has to disarm the fallback, not merely set the initial value.
	time.Sleep(300 * time.Millisecond)
	if got := tracker.GetPollMode(); got != PollModeFast {
		t.Errorf("tracker demoted to %v while the workspace is focused, want %v", got, PollModeFast)
	}
}

// TestRescanTrackerKeepsGraceWhenGatewayNeverPushed is the other half: with no
// prior push there is nothing to inherit, so the grace demotion must still own
// the tracker.
func TestRescanTrackerKeepsGraceWhenGatewayNeverPushed(t *testing.T) {
	isolateTestGitEnv(t)
	taskRoot := t.TempDir()
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	if err := os.Rename(repoDir, filepath.Join(taskRoot, "solo")); err != nil {
		t.Fatalf("place solo: %v", err)
	}

	mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
	t.Cleanup(func() { mgr.workspaceTracker.Stop() })

	tracker := mgr.newTrackerForRepo(filepath.Join(taskRoot, "solo"), "solo")
	tracker.filePollInterval = 30 * time.Second
	tracker.gitPollInterval = 30 * time.Second
	tracker.pollModeGrace = 50 * time.Millisecond
	tracker.Start(context.Background())
	t.Cleanup(tracker.Stop)

	waitForPollMode(t, tracker, PollModeSlow, 10*time.Second)
}
