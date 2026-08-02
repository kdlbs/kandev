package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// placeRepo materialises a git repo as a child of taskRoot under the given name.
func placeRepo(t *testing.T, taskRoot, name string) string {
	t.Helper()
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	dest := filepath.Join(taskRoot, name)
	if err := os.Rename(repoDir, dest); err != nil {
		t.Fatalf("place %s: %v", name, err)
	}
	return dest
}

func stopManagerTrackers(t *testing.T, m *Manager) {
	t.Helper()
	t.Cleanup(func() {
		root, trackers := m.snapshotTrackers()
		if root != nil {
			root.Stop()
		}
		for _, tr := range trackers {
			tr.Stop()
		}
	})
}

// TestManagerRescanKeepsFocusedWorkspaceFast is the manager-level regression
// test for review feedback on #2113: the user focuses a task, a rescan then
// adds a repository, and no further gateway push follows because the session's
// own mode never changed. Without inheritance the new tracker would sit at its
// construction default and be demoted by the grace timer while the workspace
// is still being viewed.
func TestManagerRescanKeepsFocusedWorkspaceFast(t *testing.T) {
	isolateTestGitEnv(t)
	taskRoot := t.TempDir()
	placeRepo(t, taskRoot, "frontend")
	placeRepo(t, taskRoot, "backend")

	mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
	stopManagerTrackers(t, mgr)

	// The user focuses the task: the gateway pushes fast, once.
	mgr.SetWorkspacePollMode(context.Background(), PollModeFast)

	// A repository appears afterwards and a rescan picks it up. The gateway
	// stays silent — the session's mode did not change.
	placeRepo(t, taskRoot, "worker")
	if err := mgr.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatalf("RescanRepositories: %v", err)
	}

	_, trackers := mgr.snapshotTrackers()
	var worker *WorkspaceTracker
	for _, tr := range trackers {
		if tr.RepositoryName() == "worker" {
			worker = tr
			break
		}
	}
	if worker == nil {
		t.Fatalf("rescan did not create a tracker for the new repository; got %d trackers", len(trackers))
	}

	if got := worker.GetPollMode(); got != PollModeFast {
		t.Errorf("rescan-created tracker = %v, want %v — it did not inherit the focused mode", got, PollModeFast)
	}
	// Deterministic: with the push recorded and the timer cleared, no elapsed
	// time can demote it, so there is nothing to wait for.
	assertGraceDisarmed(t, worker)
}

// TestManagerRescanKeepsGraceWhenNeverFocused is the other half: with no prior
// push there is nothing to inherit, so the grace demotion must still own
// rescan-created trackers.
func TestManagerRescanKeepsGraceWhenNeverFocused(t *testing.T) {
	isolateTestGitEnv(t)
	taskRoot := t.TempDir()
	placeRepo(t, taskRoot, "frontend")
	placeRepo(t, taskRoot, "backend")

	mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
	stopManagerTrackers(t, mgr)

	placeRepo(t, taskRoot, "worker")
	if err := mgr.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatalf("RescanRepositories: %v", err)
	}

	_, trackers := mgr.snapshotTrackers()
	for _, tr := range trackers {
		if tr.RepositoryName() != "worker" {
			continue
		}
		// Assert the fallback is still in charge rather than waiting out the
		// production grace period: no push recorded and the timer armed is
		// exactly the state the demotion fires from. That it does fire is
		// covered by TestPollModeGrace_DemotesWhenNoPushArrives.
		tr.pollModeMu.RLock()
		pushed, timer := tr.pollModePushed, tr.pollModeGraceTimer
		tr.pollModeMu.RUnlock()
		if pushed {
			t.Error("tracker recorded a push although the gateway never pushed for this workspace")
		}
		if timer == nil {
			t.Error("grace timer not armed, so an unwatched rescan tracker would poll fast forever")
		}
		return
	}
	t.Fatal("rescan did not create a tracker for the new repository")
}

// TestManagerRescanIgnoresInvalidPollModePush covers the gap between rejecting a
// mode and recording it. Every tracker already refuses an invalid mode, so a
// malformed push is harmless to the trackers that exist — but if the Manager
// stored it anyway, trackers created afterwards would inherit the garbage and
// sit on their construction default, indistinguishable from a workspace the
// gateway never spoke for. The last valid push has to survive.
func TestManagerRescanIgnoresInvalidPollModePush(t *testing.T) {
	isolateTestGitEnv(t)
	taskRoot := t.TempDir()
	placeRepo(t, taskRoot, "frontend")

	mgr := NewManager(&config.InstanceConfig{WorkDir: taskRoot}, newTestLogger(t))
	stopManagerTrackers(t, mgr)

	// The gateway focuses the workspace, then sends something malformed.
	mgr.SetWorkspacePollMode(context.Background(), PollModeSlow)
	mgr.SetWorkspacePollMode(context.Background(), PollMode("turbo"))

	mgr.repoTrackersMu.RLock()
	stored, ok := mgr.workspacePollMode, mgr.workspacePollModeSet
	mgr.repoTrackersMu.RUnlock()
	if !ok {
		t.Fatal("the invalid push cleared the record of the valid one")
	}
	if stored != PollModeSlow {
		t.Errorf("stored mode = %v, want %v — the invalid push overwrote it", stored, PollModeSlow)
	}

	// A repository added afterwards must inherit the last valid mode.
	placeRepo(t, taskRoot, "worker")
	if err := mgr.RescanRepositories(context.Background(), taskRoot); err != nil {
		t.Fatalf("RescanRepositories: %v", err)
	}

	_, trackers := mgr.snapshotTrackers()
	var worker *WorkspaceTracker
	for _, tr := range trackers {
		if tr.RepositoryName() == "worker" {
			worker = tr
			break
		}
	}
	if worker == nil {
		t.Fatalf("rescan did not create a tracker for the new repository; got %d trackers", len(trackers))
	}
	if got := worker.GetPollMode(); got != PollModeSlow {
		t.Errorf("new tracker poll mode = %v, want %v — it did not inherit the last valid push", got, PollModeSlow)
	}
}
