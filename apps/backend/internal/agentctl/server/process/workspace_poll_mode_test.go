package process

import (
	"testing"
	"time"
)

func TestPollMode_IsValid(t *testing.T) {
	cases := []struct {
		mode  PollMode
		valid bool
	}{
		{PollModeFast, true},
		{PollModeSlow, true},
		{PollModePaused, true},
		{PollMode(""), false},
		{PollMode("turbo"), false},
	}
	for _, tc := range cases {
		if got := tc.mode.IsValid(); got != tc.valid {
			t.Errorf("PollMode(%q).IsValid() = %v, want %v", tc.mode, got, tc.valid)
		}
	}
}

func TestNewWorkspaceTracker_DefaultsToFast(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Fresh agentctl instances start in fast mode — the gateway pushes
	// slow/paused once it determines no client is actively watching.
	// Defaulting to fast avoids a startup window where workspace changes
	// would go undetected for up to 30s.
	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("default poll mode = %q, want %q", got, PollModeFast)
	}
}

func TestSetPollMode_NoOpOnSameMode(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))

	// Drain any signal that might already be in the channels
	for _, ch := range []chan struct{}{wt.monitorModeChanged, wt.gitPollModeChanged} {
		select {
		case <-ch:
		default:
		}
	}

	// Setting the same mode (fast — the default) should not push a signal to either channel
	wt.SetPollMode(PollModeFast)
	for name, ch := range map[string]chan struct{}{
		"monitor": wt.monitorModeChanged,
		"gitPoll": wt.gitPollModeChanged,
	} {
		select {
		case <-ch:
			t.Errorf("SetPollMode with current mode should not signal %s channel", name)
		case <-time.After(20 * time.Millisecond):
			// expected: no signal
		}
	}
}

func TestSetPollMode_SignalsOnTransition(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	// Drain both channels
	for _, ch := range []chan struct{}{wt.monitorModeChanged, wt.gitPollModeChanged} {
		select {
		case <-ch:
		default:
		}
	}

	// Start at slow so fast is a real transition (default is fast).
	wt.SetPollMode(PollModeSlow)
	for _, ch := range []chan struct{}{wt.monitorModeChanged, wt.gitPollModeChanged} {
		select {
		case <-ch:
		default:
		}
	}
	wt.SetPollMode(PollModeFast)
	// Both loops must wake — each channel should receive a signal
	for name, ch := range map[string]chan struct{}{
		"monitor": wt.monitorModeChanged,
		"gitPoll": wt.gitPollModeChanged,
	} {
		select {
		case <-ch:
			// expected
		case <-time.After(50 * time.Millisecond):
			t.Fatalf("SetPollMode transition did not signal %s channel", name)
		}
	}

	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("after SetPollMode(fast) GetPollMode = %q, want %q", got, PollModeFast)
	}
}

func TestSetPollMode_RejectsInvalidMode(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))
	wt.SetPollMode(PollMode("nonsense"))
	// Default is fast; invalid mode should not mutate.
	if got := wt.GetPollMode(); got != PollModeFast {
		t.Errorf("invalid mode mutated state: GetPollMode = %q, want unchanged %q", got, PollModeFast)
	}
}

func TestPollIntervals(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))

	cases := []struct {
		mode       PollMode
		wantFile   time.Duration
		wantGit    time.Duration
		wantPaused bool
	}{
		{PollModeFast, DefaultFilePollInterval, DefaultGitPollInterval, false},
		{PollModeSlow, defaultSlowPollInterval, defaultSlowPollInterval, false},
		{PollModePaused, pausedTickInterval, pausedTickInterval, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			file, git, paused := wt.pollIntervals(tc.mode)
			if file != tc.wantFile {
				t.Errorf("file interval = %v, want %v", file, tc.wantFile)
			}
			if git != tc.wantGit {
				t.Errorf("git interval = %v, want %v", git, tc.wantGit)
			}
			if paused != tc.wantPaused {
				t.Errorf("paused = %v, want %v", paused, tc.wantPaused)
			}
		})
	}
}

// TestSetPollMode_SignalChannelDoesNotBlock verifies that calling SetPollMode
// repeatedly without anyone reading the channel does not deadlock the caller.
// The channel is buffered(1) and the send is non-blocking.
func TestSetPollMode_SignalChannelDoesNotBlock(t *testing.T) {
	isolateTestGitEnv(t)
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	wt := NewWorkspaceTracker(repoDir, newTestLogger(t))

	done := make(chan struct{})
	go func() {
		// Alternate modes so each call is a real transition. With no reader,
		// only the first send fills the buffer; subsequent sends drop on the floor.
		modes := []PollMode{PollModeFast, PollModeSlow, PollModeFast, PollModePaused, PollModeFast}
		for _, m := range modes {
			wt.SetPollMode(m)
		}
		close(done)
	}()

	select {
	case <-done:
		// expected: SetPollMode never blocks
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SetPollMode blocked when channel buffer was full")
	}
}
