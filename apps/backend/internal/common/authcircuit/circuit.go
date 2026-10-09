// Package authcircuit provides a small, storage-agnostic circuit-breaker
// value type shared by integrations (GitHub PR-watch polling, workflow-sync)
// that must stop calling a third-party API once its credential or
// configuration is permanently broken, and resume promptly once the
// credential/configuration changes rather than waiting out a fixed timer.
//
// A permanent (auth/config) failure is fundamentally different from a
// transient one (network blip, 5xx, rate limiting): retrying a transient
// failure sooner might succeed, but retrying an auth/config failure will
// keep failing identically until a human fixes the credential or config —
// continuing to call the provider on the old schedule only wastes quota and
// produces log noise. State captures just enough to make that distinction
// durable (when embedded in a caller's own persisted row) without owning
// any I/O itself.
package authcircuit

import (
	"math"
	"math/rand"
	"time"
)

// FailureClass categorizes why an integration call failed, so the caller can
// decide whether continued retries are worthwhile.
type FailureClass string

const (
	// FailureClassNone means the last attempt succeeded (or none has run
	// yet). No backoff is scheduled.
	FailureClassNone FailureClass = ""
	// FailureClassTransient marks a failure that may resolve on its own —
	// network errors, 5xx responses, secondary rate limiting. Retried on a
	// short, bounded schedule.
	FailureClassTransient FailureClass = "transient"
	// FailureClassAuth marks an authentication failure (expired, revoked, or
	// invalid credential). Only a credential change should be able to
	// shorten the backoff; the fingerprint-reset path exists for exactly
	// this.
	FailureClassAuth FailureClass = "auth"
	// FailureClassConfig marks a caller-configuration failure (bad
	// repository/branch/path, integration not configured). Only a
	// configuration change should shorten the backoff.
	FailureClassConfig FailureClass = "config"
)

// Backoff bounds the exponential delay schedule used between retries for one
// failure class.
type Backoff struct {
	// Base is the delay after the first failure.
	Base time.Duration
	// Max caps the delay regardless of how many consecutive failures have
	// occurred, so a long-broken integration still gets probed periodically
	// (bounded by JitterFraction) rather than backing off forever.
	Max time.Duration
	// JitterFraction adds up to this fraction of the computed delay as
	// additional random delay, so many circuits opened at the same moment
	// (e.g. a credential rotation that breaks several workspaces at once)
	// don't all retry in lockstep.
	JitterFraction float64
}

// delay computes the backoff for the given 1-indexed consecutive-failure
// count. rng must return a value in [0, 1).
func (b Backoff) delay(consecutiveFailures int, rng func() float64) time.Duration {
	if consecutiveFailures < 1 {
		consecutiveFailures = 1
	}
	// consecutiveFailures-1 so the first failure uses Base exactly.
	shift := consecutiveFailures - 1
	if shift > 32 { // guard against overflow on a pathologically long streak
		shift = 32
	}
	scaled := float64(b.Base) * math.Pow(2, float64(shift))
	if scaled <= 0 || scaled > float64(b.Max) {
		scaled = float64(b.Max)
	}
	if b.JitterFraction > 0 {
		scaled += scaled * b.JitterFraction * rng()
	}
	d := time.Duration(scaled)
	if d > b.Max {
		d = b.Max
	}
	if d < b.Base {
		d = b.Base
	}
	return d
}

// TransientBackoff is the default schedule for transient failures: short
// enough that a real blip recovers quickly, capped so an unattended outage
// doesn't spin ever-longer.
var TransientBackoff = Backoff{Base: 30 * time.Second, Max: 10 * time.Minute, JitterFraction: 0.25}

// PermanentBackoff is the default schedule for auth/config failures. Its
// cap is intentionally much longer than TransientBackoff's — nothing about
// waiting longer fixes a bad credential — but it still periodically probes
// (bounded by Max) in case an external fix (e.g. GitHub-side token renewal)
// happened without Kandev observing a fingerprint change.
var PermanentBackoff = Backoff{Base: 2 * time.Minute, Max: 6 * time.Hour, JitterFraction: 0.25}

// BackoffFor returns the schedule to use for the given failure class.
func BackoffFor(class FailureClass) Backoff {
	if class == FailureClassAuth || class == FailureClassConfig {
		return PermanentBackoff
	}
	return TransientBackoff
}

// State is the persisted circuit-breaker state for one integration target
// (e.g. one workspace's GitHub connection, or one workflow-sync config).
// Callers embed this by value in their own row/config struct and own its
// persistence; this package never performs I/O.
type State struct {
	FailureClass        FailureClass `json:"failure_class,omitempty"`
	ConsecutiveFailures int          `json:"consecutive_failures,omitempty"`
	NextRetryAt         *time.Time   `json:"next_retry_at,omitempty"`
	// Fingerprint is the last-observed opaque credential/config fingerprint.
	// Never contains secrets — callers must derive it from generation
	// counters, status enums, or content hashes only.
	Fingerprint string `json:"-"`
}

// Open reports whether the circuit is currently open (skip calling the
// provider) at the given time.
func (s State) Open(now time.Time) bool {
	return s.NextRetryAt != nil && now.Before(*s.NextRetryAt)
}

// RecordFailure classifies the failure, increments the consecutive-failure
// count, and schedules NextRetryAt using the bounded exponential+jitter
// schedule for that class. rng is called for jitter; pass nil in production
// to use math/rand, or a deterministic stub in tests.
func (s *State) RecordFailure(now time.Time, class FailureClass, rng func() float64) {
	if rng == nil {
		rng = rand.Float64
	}
	s.ConsecutiveFailures++
	s.FailureClass = class
	delay := BackoffFor(class).delay(s.ConsecutiveFailures, rng)
	next := now.Add(delay)
	s.NextRetryAt = &next
}

// RecordSuccess clears all failure/backoff state. Fingerprint is left
// untouched — it is only ever advanced by ResetIfFingerprintChanged or by
// the caller directly, since it tracks identity, not outcome.
func (s *State) RecordSuccess() {
	s.FailureClass = FailureClassNone
	s.ConsecutiveFailures = 0
	s.NextRetryAt = nil
}

// ResetIfFingerprintChanged clears failure/backoff state and reports true
// when newFingerprint differs from the previously observed one. An empty
// newFingerprint is treated as "unknown" and never triggers a reset (a
// caller that cannot currently compute a fingerprint must not accidentally
// clear an open circuit). The very first observed fingerprint (transition
// from "" to non-empty) updates Fingerprint but is not itself treated as a
// change, since there is no prior value to have differed from.
func (s *State) ResetIfFingerprintChanged(newFingerprint string) bool {
	if newFingerprint == "" {
		return false
	}
	if newFingerprint == s.Fingerprint {
		return false
	}
	firstObservation := s.Fingerprint == ""
	s.Fingerprint = newFingerprint
	if firstObservation {
		return false
	}
	s.RecordSuccess()
	return true
}
