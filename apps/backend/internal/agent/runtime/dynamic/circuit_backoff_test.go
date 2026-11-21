package dynamic

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type backoffClock struct{ now time.Time }

func (c *backoffClock) Now() time.Time { return c.now }

func newBackoffRegistry(clock *backoffClock) *CircuitRegistry {
	return NewCircuitRegistry(WithCircuitClock(clock.Now))
}

func circuitUntil(t *testing.T, registry *CircuitRegistry, key string) time.Time {
	t.Helper()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry, ok := registry.circuits[key]
	if !ok {
		t.Fatalf("circuit %s missing", key)
	}
	return entry.until
}

func circuitFailures(registry *CircuitRegistry, key string) int {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.circuits[key].failures
}

func rateLimitedOpen(now time.Time) FailureOpen {
	return FailureOpen{Code: routingerr.CodeRateLimited, Now: now}
}

// A failed probe advances the count. A successful probe closes the circuit
// but keeps the count, so the next failure continues the ladder; only a clean
// turn completion resets it.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
func TestCircuitBackoffProbeOutcomesAndCleanTurnReset(t *testing.T) {
	clock := &backoffClock{now: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	registry := newBackoffRegistry(clock)
	registry.OpenFailure("binding", rateLimitedOpen(clock.now))
	clock.now = clock.now.Add(2 * time.Minute)

	lease, ok := registry.AcquireProbe("binding", time.Second)
	if !ok {
		t.Fatal("AcquireProbe failed after expiry")
	}
	registry.ReleaseProbe(lease, false, circuitBackoff)
	if got := circuitFailures(registry, "binding"); got != 2 {
		t.Fatalf("failures after a failed probe = %d, want 2", got)
	}
	if got, want := circuitUntil(t, registry, "binding"), clock.now.Add(5*time.Minute); !got.Equal(want) {
		t.Fatalf("failed probe until = %v, want %v", got, want)
	}

	clock.now = clock.now.Add(6 * time.Minute)
	lease, ok = registry.AcquireProbe("binding", time.Second)
	if !ok {
		t.Fatal("AcquireProbe failed after the second expiry")
	}
	registry.ReleaseProbe(lease, true, circuitBackoff)
	if registry.IsOpen("binding", clock.now) {
		t.Fatal("a successful probe left the circuit open")
	}
	if got := circuitFailures(registry, "binding"); got != 2 {
		t.Fatalf("failures after a successful probe = %d, want 2 (kept)", got)
	}
	registry.OpenFailure("binding", rateLimitedOpen(clock.now))
	if got, want := circuitUntil(t, registry, "binding"), clock.now.Add(15*time.Minute); !got.Equal(want) {
		t.Fatalf("failure after a successful probe until = %v, want %v", got, want)
	}

	if !registry.ResetFailures("binding") || circuitFailures(registry, "binding") != 0 {
		t.Fatal("a clean turn did not reset the failure count")
	}
	clock.now = clock.now.Add(time.Hour)
	registry.OpenFailure("binding", rateLimitedOpen(clock.now))
	if got, want := circuitUntil(t, registry, "binding"), clock.now.Add(time.Minute); !got.Equal(want) {
		t.Fatalf("first failure after a clean turn until = %v, want %v", got, want)
	}
}

// A provider reset time only ever extends the ladder deadline; an inferred
// one is capped at 24 hours while an explicit one stays authoritative.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
func TestCircuitBackoffResetHints(t *testing.T) {
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		hint     time.Duration
		inferred bool
		want     time.Duration
	}{
		{name: "earlier hint keeps the ladder", hint: 30 * time.Second, want: time.Minute},
		{name: "later explicit hint wins", hint: 3 * time.Hour, want: 3 * time.Hour},
		{name: "explicit hint beyond a day wins", hint: 72 * time.Hour, want: 72 * time.Hour},
		{name: "inferred hint beyond a day is capped", hint: 30 * time.Hour, inferred: true, want: 24 * time.Hour},
		{name: "inferred hint within a day wins", hint: 6 * time.Hour, inferred: true, want: 6 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clock := &backoffClock{now: start}
			registry := newBackoffRegistry(clock)
			hint := start.Add(tc.hint)
			registry.OpenFailure("binding", FailureOpen{
				Code: routingerr.CodeQuotaLimited, Now: start, ResetHint: &hint, ResetHintInferred: tc.inferred,
			})
			if got := circuitUntil(t, registry, "binding"); !got.Equal(start.Add(tc.want)) {
				t.Fatalf("until = %v after the failure, want %v", got.Sub(start), tc.want)
			}
		})
	}
}

// An owner-release hold is never replaced by a timed open.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
func TestCircuitBackoffNeverReplacesAHold(t *testing.T) {
	clock := &backoffClock{now: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	registry := newBackoffRegistry(clock)
	if _, err := registry.Hold("binding", routingerr.CodeAccountBlocked); err != nil {
		t.Fatalf("Hold: %v", err)
	}
	registry.OpenFailure("binding", rateLimitedOpen(clock.now))
	if !registry.IsHeld("binding") || circuitFailures(registry, "binding") != 0 {
		t.Fatal("a timed open replaced the owner-release hold")
	}
}
