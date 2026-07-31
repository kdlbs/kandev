package process

import (
	"context"
	"testing"
	"time"
)

// waitForPollMode polls GetPollMode until it matches want or the deadline
// passes. Used instead of a fixed sleep because the grace demotion fires on a
// timer goroutine, and CI hosts under load can delay it well past its nominal
// deadline.
func waitForPollMode(t *testing.T, wt *WorkspaceTracker, want PollMode, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := wt.GetPollMode(); got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("poll mode = %v after %v, want %v", wt.GetPollMode(), timeout, want)
}

// TestPollModeGrace_DemotesWhenNoPushArrives covers the leak this mechanism
// exists to close: the gateway only pushes a mode for workspaces a client
// focuses or subscribes to, and it deliberately skips the paused push for a
// workspace it has never pushed to. Without the grace demotion such a tracker
// stays at the fast 2s/3s cadence for the life of the process.
func TestPollModeGrace_DemotesWhenNoPushArrives(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	// Long poll intervals keep the loops from doing real git work during the
	// test; we are asserting on the mode, not on scan output.
	wt.filePollInterval = 30 * time.Second
	wt.gitPollInterval = 30 * time.Second
	wt.pollModeGrace = 50 * time.Millisecond

	if got := wt.GetPollMode(); got != PollModeFast {
		t.Fatalf("construction mode = %v, want %v", got, PollModeFast)
	}

	wt.Start(context.Background())
	defer wt.Stop()

	waitForPollMode(t, wt, PollModeSlow, 10*time.Second)
}

// TestPollModeGrace_ExplicitPushDisarmsDemotion verifies the gateway wins: once
// it has spoken for a workspace, the fallback must never override it.
func TestPollModeGrace_ExplicitPushDisarmsDemotion(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.filePollInterval = 30 * time.Second
	wt.gitPollInterval = 30 * time.Second
	wt.pollModeGrace = 50 * time.Millisecond

	wt.Start(context.Background())
	defer wt.Stop()

	// A focused workspace: the gateway pushes fast right after the instance
	// comes up. The tracker must still be fast well after the grace deadline.
	wt.SetPollMode(PollModeFast)
	time.Sleep(300 * time.Millisecond)

	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("poll mode = %v after an explicit fast push, want %v", got, PollModeFast)
	}
}

// TestPollModeGrace_NoOpPushStillDisarms pins the subtle case: pushing the mode
// the tracker is already in changes no cadence, but it does prove the gateway
// is managing this workspace — which is precisely what the grace demotion
// exists to detect the absence of. Recording the push must therefore happen
// before SetPollMode's same-mode early return.
func TestPollModeGrace_NoOpPushStillDisarms(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.filePollInterval = 30 * time.Second
	wt.gitPollInterval = 30 * time.Second
	wt.pollModeGrace = 50 * time.Millisecond

	wt.Start(context.Background())
	defer wt.Stop()

	// Same mode the tracker was constructed with — a no-op for the cadence.
	wt.SetPollMode(PollModeFast)
	time.Sleep(300 * time.Millisecond)

	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("poll mode = %v after a same-mode push, want %v — the push was not recorded", got, PollModeFast)
	}
}

// TestPollModeGrace_NotArmedWhenNeverStarted guards against a tracker that is
// constructed but never started leaving a live timer (and therefore a
// goroutine reference) behind.
func TestPollModeGrace_NotArmedWhenNeverStarted(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.pollModeGrace = 50 * time.Millisecond

	time.Sleep(300 * time.Millisecond)

	wt.pollModeMu.RLock()
	armed := wt.pollModeGraceTimer != nil
	wt.pollModeMu.RUnlock()
	if armed {
		t.Error("grace timer armed on a tracker that was never started")
	}
	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("poll mode = %v on an unstarted tracker, want %v", got, PollModeFast)
	}
}
