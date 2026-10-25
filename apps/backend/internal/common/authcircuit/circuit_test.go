package authcircuit

import (
	"testing"
	"time"
)

func noJitter() float64 { return 0 }

func TestBackoffDelayGrowsExponentiallyAndCaps(t *testing.T) {
	b := Backoff{Base: 1 * time.Second, Max: 10 * time.Second, JitterFraction: 0}
	got := []time.Duration{
		b.delay(1, noJitter),
		b.delay(2, noJitter),
		b.delay(3, noJitter),
		b.delay(4, noJitter), // 8s, still under cap
		b.delay(5, noJitter), // would be 16s, capped to 10s
		b.delay(100, noJitter),
	}
	want := []time.Duration{
		1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		10 * time.Second, 10 * time.Second,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("delay(%d) = %v, want %v", i+1, got[i], want[i])
		}
	}
}

func TestBackoffDelayNeverBelowBase(t *testing.T) {
	b := Backoff{Base: 5 * time.Second, Max: 60 * time.Second, JitterFraction: 0}
	if got := b.delay(0, noJitter); got != b.Base {
		t.Fatalf("delay(0) = %v, want base %v (non-positive attempt clamps to 1)", got, b.Base)
	}
	if got := b.delay(-3, noJitter); got != b.Base {
		t.Fatalf("delay(-3) = %v, want base %v", got, b.Base)
	}
}

func TestBackoffJitterStaysWithinBounds(t *testing.T) {
	b := Backoff{Base: 10 * time.Second, Max: 100 * time.Second, JitterFraction: 0.25}
	for _, rngVal := range []float64{0, 0.5, 0.999} {
		got := b.delay(1, func() float64 { return rngVal })
		min := b.Base
		max := b.Base + time.Duration(float64(b.Base)*0.25)
		if got < min || got > max {
			t.Fatalf("delay with rng=%v = %v, want in [%v, %v]", rngVal, got, min, max)
		}
	}
}

func TestBackoffForSelectsScheduleByClass(t *testing.T) {
	if BackoffFor(FailureClassTransient) != TransientBackoff {
		t.Fatal("transient class should use TransientBackoff")
	}
	if BackoffFor(FailureClassNone) != TransientBackoff {
		t.Fatal("none class should default to TransientBackoff")
	}
	if BackoffFor(FailureClassAuth) != PermanentBackoff {
		t.Fatal("auth class should use PermanentBackoff")
	}
	if BackoffFor(FailureClassConfig) != PermanentBackoff {
		t.Fatal("config class should use PermanentBackoff")
	}
}

func TestStateOpenReflectsNextRetryAt(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var s State
	if s.Open(now) {
		t.Fatal("zero-value state must not be open")
	}
	future := now.Add(time.Minute)
	s.NextRetryAt = &future
	if !s.Open(now) {
		t.Fatal("state with future NextRetryAt must be open")
	}
	if s.Open(future.Add(time.Second)) {
		t.Fatal("state must not be open once now has passed NextRetryAt")
	}
}

func TestRecordFailureIncrementsAndSchedulesBackoff(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var s State
	s.RecordFailure(now, FailureClassAuth, noJitter)
	if s.ConsecutiveFailures != 1 {
		t.Fatalf("ConsecutiveFailures = %d, want 1", s.ConsecutiveFailures)
	}
	if s.FailureClass != FailureClassAuth {
		t.Fatalf("FailureClass = %q, want auth", s.FailureClass)
	}
	if s.NextRetryAt == nil || !s.NextRetryAt.Equal(now.Add(PermanentBackoff.Base)) {
		t.Fatalf("NextRetryAt = %v, want %v", s.NextRetryAt, now.Add(PermanentBackoff.Base))
	}
	if !s.Open(now) {
		t.Fatal("state must be open immediately after RecordFailure")
	}

	s.RecordFailure(now, FailureClassAuth, noJitter)
	if s.ConsecutiveFailures != 2 {
		t.Fatalf("ConsecutiveFailures = %d, want 2", s.ConsecutiveFailures)
	}
	wantSecond := now.Add(PermanentBackoff.Base * 2)
	if !s.NextRetryAt.Equal(wantSecond) {
		t.Fatalf("second NextRetryAt = %v, want %v", s.NextRetryAt, wantSecond)
	}
}

func TestRecordSuccessClearsFailureState(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := State{Fingerprint: "keep-me"}
	s.RecordFailure(now, FailureClassTransient, noJitter)
	s.RecordSuccess()
	if s.FailureClass != FailureClassNone || s.ConsecutiveFailures != 0 || s.NextRetryAt != nil {
		t.Fatalf("RecordSuccess left state = %+v, want all cleared", s)
	}
	if s.Fingerprint != "keep-me" {
		t.Fatal("RecordSuccess must not touch Fingerprint")
	}
}

func TestResetIfFingerprintChanged(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty fingerprint never resets", func(t *testing.T) {
		s := State{}
		s.RecordFailure(now, FailureClassAuth, noJitter)
		if changed := s.ResetIfFingerprintChanged(""); changed {
			t.Fatal("empty fingerprint must not report a change")
		}
		if !s.Open(now) {
			t.Fatal("circuit must remain open when fingerprint is unknown")
		}
	})

	t.Run("first observation records but does not reset", func(t *testing.T) {
		s := State{}
		s.RecordFailure(now, FailureClassAuth, noJitter)
		if changed := s.ResetIfFingerprintChanged("gen-1"); changed {
			t.Fatal("first observed fingerprint must not itself report a change")
		}
		if s.Fingerprint != "gen-1" {
			t.Fatalf("Fingerprint = %q, want gen-1", s.Fingerprint)
		}
		if !s.Open(now) {
			t.Fatal("first observation must not clear an already-open circuit")
		}
	})

	t.Run("unchanged fingerprint does not reset", func(t *testing.T) {
		s := State{Fingerprint: "gen-1"}
		s.RecordFailure(now, FailureClassAuth, noJitter)
		if changed := s.ResetIfFingerprintChanged("gen-1"); changed {
			t.Fatal("identical fingerprint must not report a change")
		}
		if !s.Open(now) {
			t.Fatal("circuit must remain open when the fingerprint has not changed")
		}
	})

	t.Run("changed fingerprint resets the circuit", func(t *testing.T) {
		s := State{Fingerprint: "gen-1"}
		s.RecordFailure(now, FailureClassAuth, noJitter)
		if !s.Open(now) {
			t.Fatal("precondition: circuit should be open before the reset")
		}
		if changed := s.ResetIfFingerprintChanged("gen-2"); !changed {
			t.Fatal("changed fingerprint must report a change")
		}
		if s.Fingerprint != "gen-2" {
			t.Fatalf("Fingerprint = %q, want gen-2", s.Fingerprint)
		}
		if s.Open(now) {
			t.Fatal("circuit must be closed immediately after a fingerprint change")
		}
	})
}
