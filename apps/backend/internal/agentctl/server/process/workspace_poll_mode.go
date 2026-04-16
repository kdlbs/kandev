package process

import (
	"time"

	"go.uber.org/zap"
)

// PollMode controls how aggressively WorkspaceTracker polls for workspace changes.
//
// The mode is driven by the gateway based on UI subscription/focus state for the
// session(s) backed by this workspace. See plan: focus-gated git polling.
type PollMode string

const (
	// PollModeFast is the default fast polling rate used when a UI client is
	// actively focused on a session in this workspace (task details page, task
	// panel modal). Matches the historical polling rate prior to the gating change.
	PollModeFast PollMode = "fast"

	// PollModeSlow is used when sessions in this workspace are subscribed but not
	// focused — typically a sidebar card showing a diff badge. Polls infrequently
	// so the badge can stay roughly fresh without burning CPU on every retained
	// task worktree.
	PollModeSlow PollMode = "slow"

	// PollModePaused suppresses git scans entirely. The polling goroutines stay
	// alive but their tick bodies are no-ops, so the next "fast" or "slow" mode
	// change resumes work within one tick interval (no goroutine restart).
	PollModePaused PollMode = "paused"
)

// IsValid returns true if m is one of the known poll modes.
func (m PollMode) IsValid() bool {
	switch m {
	case PollModeFast, PollModeSlow, PollModePaused:
		return true
	default:
		return false
	}
}

// Slow-mode interval for both monitorLoop (untracked-file scan) and
// pollGitChanges (HEAD/branch/index check). 30 seconds is the default.
// Smaller = fresher sidebar badges but more CPU; larger = staler badges.
const defaultSlowPollInterval = 30 * time.Second

// pausedTickInterval is how often paused-mode timers wake up to check whether
// the mode has changed. The body is a no-op so this is purely a "wake and look
// at the mode" cadence. Kept relatively long because real wake-ups also happen
// via SetPollMode pushing on pollModeChanged.
const pausedTickInterval = 60 * time.Second

// pollIntervals returns the file-monitor and git-poll intervals for a mode,
// plus whether the mode is paused (in which case the body should skip git work).
func (wt *WorkspaceTracker) pollIntervals(mode PollMode) (filePoll, gitPoll time.Duration, paused bool) {
	switch mode {
	case PollModeFast:
		return wt.filePollInterval, wt.gitPollInterval, false
	case PollModeSlow:
		return defaultSlowPollInterval, defaultSlowPollInterval, false
	case PollModePaused:
		return pausedTickInterval, pausedTickInterval, true
	default:
		// Unknown mode: behave like slow (safe fallback).
		return defaultSlowPollInterval, defaultSlowPollInterval, false
	}
}

// GetPollMode returns the current poll mode.
func (wt *WorkspaceTracker) GetPollMode() PollMode {
	wt.pollModeMu.RLock()
	defer wt.pollModeMu.RUnlock()
	return wt.pollMode
}

// SetPollMode updates the poll mode. Calling with the current mode is a no-op.
// Transitioning into PollModeFast (from any other mode) wakes both polling loops
// immediately so the focused user sees fresh git state without waiting up to
// the slow-poll interval for the next tick.
func (wt *WorkspaceTracker) SetPollMode(mode PollMode) {
	if !mode.IsValid() {
		wt.logger.Warn("ignoring invalid poll mode", zap.String("mode", string(mode)))
		return
	}

	wt.pollModeMu.Lock()
	prev := wt.pollMode
	if prev == mode {
		wt.pollModeMu.Unlock()
		return
	}
	wt.pollMode = mode
	wt.pollModeMu.Unlock()

	// Wake both polling loops. Each has its own buffered(1) channel because
	// a single shared channel would let whichever loop selects first steal
	// the signal, leaving the other loop blocked on its old timer interval
	// (potentially 30s) before noticing the mode change. Sends are non-blocking:
	// if the buffer is full, a notification is already pending which is fine.
	for _, ch := range []chan struct{}{wt.monitorModeChanged, wt.gitPollModeChanged} {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
