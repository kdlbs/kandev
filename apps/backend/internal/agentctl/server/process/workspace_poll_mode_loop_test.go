package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// drainStream consumes everything currently buffered in a workspace stream subscription
// without blocking. Used to clear the initial snapshot before the test starts asserting.
func drainStream(sub types.WorkspaceStreamSubscriber) {
	for {
		select {
		case <-sub:
		default:
			return
		}
	}
}

// waitForFileChangeNotification reads from the subscription until it sees a
// FileChange notification or the timeout fires.
func waitForFileChangeNotification(sub types.WorkspaceStreamSubscriber, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case msg, ok := <-sub:
			if !ok {
				return false
			}
			if msg.Type == types.WorkspaceMessageTypeFileChange {
				return true
			}
		case <-deadline.C:
			return false
		}
	}
}

// TestMonitorLoop_PausedSuppressesNotifications verifies that no file-change
// notifications are emitted when the tracker is in PollModePaused, even when
// real changes happen in the workspace.
func TestMonitorLoop_PausedSuppressesNotifications(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	// Speed up the would-be-tick interval so the test runs fast. In paused
	// mode the timer fires at pausedTickInterval (60s) which is too long to
	// wait, but in fast mode it would fire at 50ms — proving paused suppresses.
	wt.filePollInterval = 50 * time.Millisecond
	wt.gitPollInterval = 50 * time.Millisecond

	wt.SetPollMode(PollModePaused)
	wt.Start(context.Background())
	defer wt.Stop()

	sub := wt.SubscribeWorkspaceStream()
	defer wt.UnsubscribeWorkspaceStream(sub)
	drainStream(sub)

	// Create an untracked file — would normally trigger a notification on the
	// next tick. In paused mode no monitor tick runs, so no notification.
	if err := os.WriteFile(filepath.Join(repoDir, "new_file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Wait long enough that several fast-mode ticks would have fired, plus
	// margin for scheduling. If paused short-circuit works, no notification.
	if got := waitForFileChangeNotification(sub, 500*time.Millisecond); got {
		t.Error("expected no file-change notification in paused mode, got one")
	}
}

// TestMonitorLoop_TransitionToFastTriggersImmediateScan verifies that switching
// from a non-fast mode into fast does not wait for the next ticker — the loop
// runs an immediate scan so the user sees fresh state right away.
func TestMonitorLoop_TransitionToFastTriggersImmediateScan(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	// Make the timer interval irrelevant by using a long fast interval — we're
	// proving the immediate-scan-on-mode-change path, not the timer.
	wt.filePollInterval = 30 * time.Second
	wt.gitPollInterval = 30 * time.Second

	wt.SetPollMode(PollModePaused)
	wt.Start(context.Background())
	defer wt.Stop()

	// Subscribe + drain the initial snapshot. Subscribing first ensures we
	// don't miss notifications fired between Start and our first wait.
	sub := wt.SubscribeWorkspaceStream()
	defer wt.UnsubscribeWorkspaceStream(sub)

	// Wait long enough for the monitorLoop goroutine to complete its initial
	// scan (updateGitStatus, updateFiles, getWorkspaceState) and enter its
	// select. Without this, the file we write below could land before the
	// initial state is captured, and the immediate-scan tick would not see
	// any state change.
	time.Sleep(300 * time.Millisecond)
	drainStream(sub)

	// Create a change so the immediate scan finds something to notify about.
	if err := os.WriteFile(filepath.Join(repoDir, "trigger.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Switch to fast — the loop should wake on pollModeChanged and run a tick
	// without waiting 30s for the timer.
	wt.SetPollMode(PollModeFast)

	if got := waitForFileChangeNotification(sub, 2*time.Second); !got {
		t.Error("expected immediate file-change notification after transitioning to fast mode")
	}
}

// TestMonitorLoop_FastPolls verifies that fast mode actually polls at its
// configured interval (smoke test for the resettable-timer path).
func TestMonitorLoop_FastPolls(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.filePollInterval = 50 * time.Millisecond
	wt.gitPollInterval = 50 * time.Millisecond

	wt.SetPollMode(PollModeFast)
	wt.Start(context.Background())
	defer wt.Stop()

	sub := wt.SubscribeWorkspaceStream()
	defer wt.UnsubscribeWorkspaceStream(sub)

	// Wait for the loop's initial state capture before writing — otherwise the
	// file write can race the initial scan and the diff is missed.
	time.Sleep(200 * time.Millisecond)
	drainStream(sub)

	if err := os.WriteFile(filepath.Join(repoDir, "tick.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if got := waitForFileChangeNotification(sub, 1*time.Second); !got {
		t.Error("expected file-change notification within 1s in fast mode")
	}
}

// TestMonitorLoop_FastToPausedStopsPolling verifies that switching from fast to
// paused stops emitting notifications for subsequent changes.
func TestMonitorLoop_FastToPausedStopsPolling(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.filePollInterval = 50 * time.Millisecond
	wt.gitPollInterval = 50 * time.Millisecond

	wt.SetPollMode(PollModeFast)
	wt.Start(context.Background())
	defer wt.Stop()

	sub := wt.SubscribeWorkspaceStream()
	defer wt.UnsubscribeWorkspaceStream(sub)

	// Wait for monitorLoop's initial scan to capture lastState before we start
	// writing files. Otherwise the file we write below could land before the
	// initial state is captured, and the first tick would not see a state change.
	time.Sleep(200 * time.Millisecond)
	drainStream(sub)

	// Confirm fast mode works first.
	if err := os.WriteFile(filepath.Join(repoDir, "before_pause.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if !waitForFileChangeNotification(sub, 1*time.Second) {
		t.Fatal("setup: expected fast mode to emit notification before pausing")
	}

	// Pause and drain any in-flight notifications.
	wt.SetPollMode(PollModePaused)
	// Give any pending tick time to complete and drain.
	time.Sleep(150 * time.Millisecond)
	drainStream(sub)

	// New change should NOT generate a notification in paused mode.
	if err := os.WriteFile(filepath.Join(repoDir, "after_pause.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if waitForFileChangeNotification(sub, 500*time.Millisecond) {
		t.Error("expected no notification after switching to paused mode")
	}
}
