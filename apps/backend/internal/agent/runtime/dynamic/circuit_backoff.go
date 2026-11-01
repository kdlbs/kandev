package dynamic

import (
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// circuitBackoff is the fixed backoff of one open while escalation is
// disabled, and the first rung of the escalation ladder.
const circuitBackoff = time.Minute

// circuitBackoffLadder is the backoff of each consecutive qualifying failure
// of one resource while escalation is enabled. The last rung is the cap.
var circuitBackoffLadder = []time.Duration{
	circuitBackoff,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	time.Hour,
	2 * time.Hour,
	4 * time.Hour,
}

// maxInferredResetHint bounds a reset time Kandev inferred rather than read
// from an explicit provider reset time.
const maxInferredResetHint = 24 * time.Hour

// escalatedBackoff returns the ladder backoff of the failures-th consecutive
// failure.
func escalatedBackoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	if failures > len(circuitBackoffLadder) {
		failures = len(circuitBackoffLadder)
	}
	return circuitBackoffLadder[failures-1]
}

// FailureOpen describes one qualifying failure that opens a circuit.
type FailureOpen struct {
	Code routingerr.Code
	// Now is the failure observation time used for the backoff deadline.
	Now time.Time
	// ResetHint is the provider reset time, when known. A later hint than the
	// backoff deadline wins.
	ResetHint *time.Time
	// ResetHintInferred marks a reset time Kandev derived without an explicit
	// provider date. While escalation is enabled it is capped at
	// maxInferredResetHint after Now.
	ResetHintInferred bool
}

// OpenFailure opens key for one qualifying failure. Disabled escalation
// applies the fixed backoff extended only by a later reset hint. Enabled
// escalation advances the consecutive failure count and applies its ladder
// rung; a failure that arrives while the same circuit is still open within its
// window belongs to the same incident and does not advance the count. A closed
// circuit keeps its count until ResetFailures, so a failure after a
// successful probe continues the ladder. An owner-release hold is never
// replaced.
func (r *CircuitRegistry) OpenFailure(key string, failure FailureOpen) {
	if key == "" {
		return
	}
	now := failure.Now
	if now.IsZero() {
		now = r.now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushPendingLocked(key)
	previous := r.circuits[key]
	if previous.held() {
		return
	}
	if !r.escalation {
		until := now.Add(circuitBackoff)
		if failure.ResetHint != nil && failure.ResetHint.After(until) {
			until = *failure.ResetHint
		}
		r.circuits[key] = circuit{state: CircuitOpen, until: until, code: failure.Code, updatedAt: now.UTC()}
		_ = r.persistSnapshotLocked(key)
		return
	}
	sameIncident := previous.state == CircuitOpen && previous.failures > 0 && now.Before(previous.until)
	failures := previous.failures + 1
	if sameIncident {
		failures = previous.failures
	}
	until := now.Add(escalatedBackoff(failures))
	if hint := boundedResetHint(failure, now); hint.After(until) {
		until = hint
	}
	if sameIncident && previous.until.After(until) {
		until = previous.until
	}
	r.circuits[key] = circuit{
		state: CircuitOpen, until: until.UTC(), code: failure.Code,
		failures: failures, updatedAt: now.UTC(),
	}
	_ = r.persistSnapshotLocked(key)
}

func boundedResetHint(failure FailureOpen, now time.Time) time.Time {
	if failure.ResetHint == nil {
		return time.Time{}
	}
	hint := *failure.ResetHint
	if failure.ResetHintInferred {
		if limit := now.Add(maxInferredResetHint); hint.After(limit) {
			hint = limit
		}
	}
	return hint
}

// ResetFailures clears the consecutive failure count of key after a turn on
// that resource completed without a provider error. The circuit state is left
// unchanged. It reports whether a count was cleared.
func (r *CircuitRegistry) ResetFailures(key string) bool {
	if key == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.circuits[key]
	if !ok || entry.failures == 0 {
		return false
	}
	r.flushPendingLocked(key)
	entry.failures = 0
	entry.updatedAt = r.now().UTC()
	r.circuits[key] = entry
	_ = r.persistSnapshotLocked(key)
	return true
}

// EscalationEnabled reports whether the registry tracks consecutive failures.
func (r *CircuitRegistry) EscalationEnabled() bool {
	return r != nil && r.escalation
}
