package routing

import (
	"math/rand"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/office/models"
)

// backoffSchedule is the exponential auto-retry ladder used when the
// provider's error carries no ResetHint. Indexed by current backoff
// step, capped at the final entry.
var backoffSchedule = []time.Duration{
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	20 * time.Minute,
	60 * time.Minute,
}

// Schedule returns the (retry_at, next_step) tuple to persist for a
// degraded provider. ResetHint short-circuits to the provider-supplied
// deadline without escalating the step. Auto-retryable codes advance
// the step and add ±25% jitter. User-action codes return a zero retry
// time so the caller can flag the row as user_action_required.
func Schedule(current models.ProviderHealth, e *routingerr.Error, now time.Time) (time.Time, int) {
	if e == nil {
		return time.Time{}, current.BackoffStep
	}
	if e.ResetHint != nil {
		return *e.ResetHint, current.BackoffStep
	}
	if !e.AutoRetryable {
		return time.Time{}, current.BackoffStep
	}
	step := current.BackoffStep
	if step < 0 {
		step = 0
	}
	idx := step
	if idx >= len(backoffSchedule) {
		idx = len(backoffSchedule) - 1
	}
	base := backoffSchedule[idx]
	jitter := jitterFor(base)
	return now.Add(base + jitter), step + 1
}

// jitterFor returns a deterministic-ish ±25% offset around base. The
// random source is the package-default rand source so production paths
// stay non-flaky; tests can seed rand themselves if they need
// deterministic output (see backoff_test.go).
func jitterFor(base time.Duration) time.Duration {
	quarter := int64(base / 4)
	if quarter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(2*quarter+1) - quarter) //nolint:gosec
}
