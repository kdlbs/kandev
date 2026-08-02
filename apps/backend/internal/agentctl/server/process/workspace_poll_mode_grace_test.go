package process

import (
	"context"
	"testing"
	"time"
)

// waitForPollMode polls GetPollMode until it matches want or the deadline
// passes. Used only for the demotion assertions, where the transition is
// inherently timer-driven; the "must NOT demote" cases assert on timer state
// instead so they need no sleeping at all.
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

// assertGraceDisarmed reports whether the fallback can still fire. Asserting on
// the timer directly is deterministic: once a push has been recorded and the
// timer cleared, no amount of elapsed time can demote the tracker, so there is
// nothing to wait for.
func assertGraceDisarmed(t *testing.T, wt *WorkspaceTracker) {
	t.Helper()
	wt.pollModeMu.RLock()
	pushed, timer := wt.pollModePushed, wt.pollModeGraceTimer
	wt.pollModeMu.RUnlock()
	if !pushed {
		t.Error("push was not recorded, so the grace demotion can still fire")
	}
	if timer != nil {
		t.Error("grace timer still armed after an explicit push")
	}
}

const (
	// graceNeverFires is long enough that the fallback cannot run mid-test.
	// Every test asserting that a push disarmed the demotion needs it: with a
	// short grace the timer can fire between Start and SetPollMode on a loaded
	// machine, and the test would still pass while exercising a different path
	// — a slow-to-fast transition instead of the case it claims to cover.
	graceNeverFires = time.Hour

	// graceFiresQuickly keeps the one test that genuinely waits for a demotion short.
	graceFiresQuickly = 50 * time.Millisecond
)

func newGraceTestTracker(t *testing.T, grace time.Duration) *WorkspaceTracker {
	t.Helper()
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	// Long poll intervals keep the loops from doing real git work; these tests
	// assert on the mode, not on scan output.
	wt.filePollInterval = 30 * time.Second
	wt.gitPollInterval = 30 * time.Second
	wt.pollModeGrace = grace
	return wt
}

// TestPollModeGrace_DemotesWhenNoPushArrives covers the leak this mechanism
// exists to close: the gateway only pushes a mode for workspaces a client
// focuses or subscribes to, and it deliberately skips the paused push for a
// workspace it has never pushed to. Without the grace demotion such a tracker
// stays at the fast 2s/3s cadence for the life of the process.
func TestPollModeGrace_DemotesWhenNoPushArrives(t *testing.T) {
	wt := newGraceTestTracker(t, graceFiresQuickly)

	if got := wt.GetPollMode(); got != PollModeFast {
		t.Fatalf("construction mode = %v, want %v", got, PollModeFast)
	}

	wt.Start(context.Background())
	defer wt.Stop()

	waitForPollMode(t, wt, PollModeSlow, 10*time.Second)
}

// TestPollModeGrace_ExplicitPushDisarmsDemotion verifies the gateway wins: once
// it has spoken for a workspace, the fallback must never override it. The push
// is a mode-changing one, and the grace is long enough that it cannot fire —
// so the slow mode asserted below can only have come from the push.
func TestPollModeGrace_ExplicitPushDisarmsDemotion(t *testing.T) {
	wt := newGraceTestTracker(t, graceNeverFires)
	wt.Start(context.Background())
	defer wt.Stop()

	wt.SetPollMode(PollModeSlow)

	assertGraceDisarmed(t, wt)
	if got := wt.GetPollMode(); got != PollModeSlow {
		t.Errorf("poll mode = %v after an explicit slow push, want %v", got, PollModeSlow)
	}
}

// TestPollModeGrace_NoOpPushStillDisarms pins the subtle case: pushing the mode
// the tracker is already in changes no cadence, but it does prove the gateway
// is managing this workspace — which is precisely what the grace demotion
// exists to detect the absence of. Recording the push must therefore happen
// before SetPollMode's same-mode early return.
func TestPollModeGrace_NoOpPushStillDisarms(t *testing.T) {
	wt := newGraceTestTracker(t, graceNeverFires)
	wt.Start(context.Background())
	defer wt.Stop()

	// Same mode the tracker was constructed with — a no-op for the cadence. The
	// long grace is what makes it one: had the fallback been able to demote
	// first, this would be a slow-to-fast transition and the test would prove
	// nothing about the same-mode path.
	wt.SetPollMode(PollModeFast)

	assertGraceDisarmed(t, wt)
	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("poll mode = %v after a no-op push, want it left at %v", got, PollModeFast)
	}
}

// TestPollModeGrace_NotArmedWhenNeverStarted guards against a tracker that is
// constructed but never started allocating a fallback timer — one nothing would
// ever disarm, since only Stop clears it.
func TestPollModeGrace_NotArmedWhenNeverStarted(t *testing.T) {
	wt := newGraceTestTracker(t, graceNeverFires)

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

// TestPollModeGrace_StopDisarmsTimer covers teardown inside the grace window:
// Stop must clear the pending timer so it cannot fire against a torn-down
// tracker and so it holds no reference past the tracker's lifetime.
func TestPollModeGrace_StopDisarmsTimer(t *testing.T) {
	// The long grace is what makes this test meaningful: the timer is certain to
	// still be pending when Stop runs.
	wt := newGraceTestTracker(t, graceNeverFires)

	wt.Start(context.Background())

	wt.pollModeMu.RLock()
	armed := wt.pollModeGraceTimer != nil
	wt.pollModeMu.RUnlock()
	if !armed {
		t.Fatal("grace timer was not armed by Start")
	}

	wt.Stop()

	wt.pollModeMu.RLock()
	timer := wt.pollModeGraceTimer
	wt.pollModeMu.RUnlock()
	if timer != nil {
		t.Error("grace timer still armed after Stop")
	}
	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("Stop changed the poll mode to %v, want it left at %v", got, PollModeFast)
	}
}
