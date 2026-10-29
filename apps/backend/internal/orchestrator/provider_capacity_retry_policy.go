package orchestrator

import (
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

const capacityMaxAttempts = 11
const capacityRetryWindow = 30 * time.Minute
const recoveryDispositionWindowExpired = "window_expired"
const recoveryDispositionRefused = "refused"
const retainedFailureScope = "turn"

// The deadline retains its monotonic component and is copied across the episode.
type transientRetryPolicy struct {
	windowEndsAt time.Time
}

func (p transientRetryPolicy) maxAttempts() int {
	if !p.windowEndsAt.IsZero() {
		return capacityMaxAttempts
	}
	return transientMaxAttempts
}

func (p transientRetryPolicy) delay(classified *routingerr.Error, attempt int, now time.Time) time.Duration {
	if p.windowEndsAt.IsZero() {
		return transientRetryDelayFor(classified, attempt, now)
	}
	// A capacity episode reuses the existing ladder. A validated short provider
	// hint can only raise the first five delays; the fixed later slots and the
	// thirty-minute deadline stay unchanged.
	if attempt <= transientMaxAttempts {
		nominal := transientRetryDelay(attempt)
		if hint := capacityResetHintDelay(classified, now); hint > nominal {
			return hint
		}
		return nominal
	}
	return capacityLongRetryDelay(attempt)
}

func capacityResetHintDelay(classified *routingerr.Error, now time.Time) time.Duration {
	if classified == nil || classified.ResetHint == nil {
		return 0
	}
	untilReset := classified.ResetHint.Sub(now)
	if untilReset < 0 || untilReset > time.Minute {
		return 0
	}
	return untilReset
}

func capacityLongRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 6:
		return 2 * time.Minute
	case 7:
		return 4 * time.Minute
	default:
		return 5 * time.Minute
	}
}

func (p transientRetryPolicy) expired(now time.Time) bool {
	return !p.windowEndsAt.IsZero() && !now.Before(p.windowEndsAt)
}

func (e *transientRetryEntry) stopDisposition(now time.Time) string {
	if e.policy.expired(now) {
		return recoveryDispositionWindowExpired
	}
	e.mu.Lock()
	started := e.started
	e.mu.Unlock()
	if started >= e.policy.maxAttempts() {
		return recoveryDispositionExhausted
	}
	return ""
}

func capacityRetryPolicy(classified *routingerr.Error, retained *retainedRuntimeRetry, previous *transientRetryEntry, now time.Time) transientRetryPolicy {
	if previous != nil {
		return previous.policy
	}
	if classified != nil && classified.Code == routingerr.CodeModelCapacity && retained != nil &&
		retained.nativeID != "" && retained.identity != ([32]byte{}) {
		return transientRetryPolicy{windowEndsAt: now.Add(capacityRetryWindow)}
	}
	return transientRetryPolicy{}
}

func (e *transientRetryEntry) pendingStopDisposition(now time.Time) string {
	if e.waitForWindow {
		return recoveryDispositionWindowExpired
	}
	return e.stopDisposition(now)
}
