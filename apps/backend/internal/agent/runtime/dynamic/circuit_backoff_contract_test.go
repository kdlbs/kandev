package dynamic

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

func TestCircuitFailureLadderAndSameIncident(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	registry := NewCircuitRegistry(WithCircuitBackoffEscalation(true), WithCircuitClock(func() time.Time { return now }))
	key := "credential:test"
	ladder := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour}
	for i, delay := range ladder {
		registry.OpenFailure(key, FailureOpen{Code: routingerr.CodeRateLimited, Now: now})
		snapshots := registry.Snapshots()
		if len(snapshots) != 1 || snapshots[0].Failures != i+1 || !snapshots[0].Until.Equal(now.Add(delay)) {
			t.Fatalf("incident %d: got %+v, want count %d and deadline %v", i+1, snapshots, i+1, now.Add(delay))
		}
		if _, blocked := registry.ActiveSuspension(key, now); !blocked {
			t.Fatal("open circuit did not block")
		}
		registry.OpenFailure(key, FailureOpen{Code: routingerr.CodeRateLimited, Now: now})
		if got := registry.Snapshots()[0].Failures; got != i+1 {
			t.Fatalf("same incident counted twice: %d", got)
		}
		now = now.Add(delay)
		if _, blocked := registry.ActiveSuspension(key, now); blocked {
			t.Fatal("expired circuit blocked at deadline")
		}
	}
	if !registry.ResetFailures(key) {
		t.Fatal("failure count was not reset")
	}
	snapshot := registry.Snapshots()[0]
	if snapshot.Failures != 0 || snapshot.State != CircuitOpen {
		t.Fatalf("reset changed circuit state: %+v", snapshot)
	}
	if registry.ResetFailures(key) {
		t.Fatal("reset of zero count reported a change")
	}
}

func TestCircuitSuspensionExpiryAndHold(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		entry   circuit
		blocked bool
	}{
		{"timed open", circuit{state: CircuitOpen, code: routingerr.CodeRateLimited, until: now.Add(time.Minute)}, true},
		{"expired open", circuit{state: CircuitOpen, code: routingerr.CodeRateLimited, until: now}, false},
		{"live probe", circuit{state: CircuitHalfOpen, code: routingerr.CodeRateLimited, until: now.Add(-time.Minute), probeUntil: now.Add(time.Second)}, true},
		{"expired probe", circuit{state: CircuitHalfOpen, code: routingerr.CodeRateLimited, until: now.Add(-time.Minute), probeUntil: now}, false},
		{"owner hold", circuit{state: CircuitOpen, code: routingerr.CodeAccountBlocked}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewCircuitRegistry()
			registry.circuits["profile:one"] = tc.entry
			_, blocked := registry.ActiveSuspension("profile:one", now)
			if blocked != tc.blocked {
				t.Fatalf("blocked=%v want %v", blocked, tc.blocked)
			}
			if registry.circuits["profile:one"] != tc.entry {
				t.Fatal("read changed circuit")
			}
		})
	}
}

func TestCircuitResetHintAndDisabledBackoff(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hint := now.Add(48 * time.Hour)
	for _, tc := range []struct {
		name                 string
		escalation, inferred bool
		want                 time.Duration
	}{
		{"disabled retains provider hint", false, true, 48 * time.Hour},
		{"inferred hint is bounded", true, true, 24 * time.Hour},
		{"explicit hint is retained", true, false, 48 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewCircuitRegistry(WithCircuitBackoffEscalation(tc.escalation))
			registry.OpenFailure("credential:hint", FailureOpen{Code: routingerr.CodeRateLimited, Now: now, ResetHint: &hint, ResetHintInferred: tc.inferred})
			got := registry.Snapshots()[0]
			if !got.Until.Equal(now.Add(tc.want)) {
				t.Fatalf("deadline=%v want %v", got.Until, now.Add(tc.want))
			}
			if !tc.escalation && got.Failures != 0 {
				t.Fatal("disabled escalation counted failure")
			}
		})
	}
}
