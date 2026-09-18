package startup

import (
	"testing"
	"time"
)

func TestOpaqueStepOmitsCountsRateAndSinceAdvance(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepPromptSeqBackfill) // registry declares opaque
	snap := r.Snapshot().Step
	if snap.Done != nil || snap.Total != nil || snap.RatePerSecond != nil ||
		snap.ETAMS != nil || snap.SinceAdvanceMS != nil {
		t.Fatalf("opaque step must omit every numeric field, got %#v", snap)
	}
	if snap.Stalled {
		t.Fatal("an opaque step must never report stalled")
	}
}

func TestCountingStepCarriesDoneNoTotal(t *testing.T) {
	r := New(nil)
	r.Set(BackingUpDatabase)
	r.BeginStep(StepDatabaseBackup)
	r.Advance(StepDatabaseBackup, 1024)
	snap := r.Snapshot().Step
	if snap.Done == nil || *snap.Done != 1024 {
		t.Fatalf("counting step should carry done=1024, got %v", snap.Done)
	}
	if snap.Total != nil {
		t.Fatal("counting step must omit total")
	}
	if snap.ETAMS != nil {
		t.Fatal("a step with no total must never carry an ETA")
	}
}

func TestCountedStepCarriesDoneAndTotal(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 10)
	r.Advance(StepStoresRepositories, 4)
	snap := r.Snapshot().Step
	if snap.Done == nil || *snap.Done != 4 {
		t.Fatalf("done = %v, want 4", snap.Done)
	}
	if snap.Total == nil || *snap.Total != 10 {
		t.Fatalf("total = %v, want 10", snap.Total)
	}
}

func TestRateOmittedBeforeFirstAdvance(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 10)
	snap := r.Snapshot().Step
	if snap.RatePerSecond != nil {
		t.Fatal("rate must be absent before the step has advanced at least once")
	}
	if snap.SinceAdvanceMS == nil {
		t.Fatal("since_advance_ms must be present for a counted step even before its first advance")
	}
}

func TestRatePresentAfterAdvanceEvenIfDecayedToZero(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 100)
	r.Advance(StepStoresRepositories, 1)

	// Force the only sample outside the 30s window and force the >=2-samples
	// branch by injecting a second, older synthetic sample directly.
	a := r.active
	a.samples = []rateSample{
		{at: time.Now().Add(-90 * time.Second), delta: 1},
		{at: time.Now().Add(-40 * time.Second), delta: 1},
	}
	a.done = 2
	snap := r.Snapshot().Step
	if snap.RatePerSecond == nil {
		t.Fatal("rate must remain present (even at zero) once the step has advanced")
	}
	if *snap.RatePerSecond != 0 {
		t.Fatalf("rate should have decayed to zero with no samples inside the window, got %v", *snap.RatePerSecond)
	}
	if snap.ETAMS != nil {
		t.Fatal("a zero rate must never carry an ETA")
	}
}

func TestRateFallsBackToStepStartBelowTwoSamples(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 1000)
	r.active.startedAt = time.Now().Add(-10 * time.Second)
	r.Advance(StepStoresRepositories, 50) // exactly one sample

	rate := r.Snapshot().Step.RatePerSecond
	if rate == nil {
		t.Fatal("expected a rate with one sample recorded")
	}
	// ~50 units over ~10s from the step's start, not from the single sample's
	// own (near-zero) age.
	if *rate <= 0 || *rate > 10 {
		t.Fatalf("rate = %v, expected roughly 5/s using step start as the older point", *rate)
	}
}

func TestETAOmittedAboveOneDay(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 1_000_000_000)
	r.active.startedAt = time.Now().Add(-10 * time.Second)
	r.Advance(StepStoresRepositories, 1) // a tiny rate projects far beyond a day

	snap := r.Snapshot().Step
	if snap.ETAMS != nil {
		t.Fatalf("eta_ms must be omitted above 86400000ms, got %v", *snap.ETAMS)
	}
}

func TestStallDetection(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 100)
	r.Advance(StepStoresRepositories, 1)

	r.active.lastAdvanceAt = time.Now().Add(-(stallThresholdMS + 1) * time.Millisecond)
	snap := r.Snapshot().Step
	if !snap.Stalled {
		t.Fatal("expected stalled=true once since_advance_ms crosses the threshold")
	}

	r.Advance(StepStoresRepositories, 1) // real progress clears the stall
	if r.Snapshot().Step.Stalled {
		t.Fatal("an advance that increases done must clear stalled immediately")
	}
}

func TestStallClockUnaffectedBySeedNoOpOrForeignAdvance(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 10)
	r.active.startedAt = time.Now().Add(-500 * time.Millisecond)
	before := *r.Snapshot().Step.SinceAdvanceMS

	r.SeedDone(StepStoresRepositories, 0) // rejected: total already set
	r.Advance(StepStoresServices, 5)      // foreign identifier
	r.Advance(StepStoresRepositories, 0)  // zero advance: no-op

	after := *r.Snapshot().Step.SinceAdvanceMS
	if after < before {
		t.Fatalf("since_advance_ms must not have reset: before=%d after=%d", before, after)
	}
}

func TestAdvanceExceedingTotalOnlyCountsRealIncreaseAsASample(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 10)
	r.Advance(StepStoresRepositories, 5)
	r.Advance(StepStoresRepositories, 999) // clamps to 10; actual increase is 5

	if got := len(r.active.samples); got != 2 {
		t.Fatalf("expected 2 recorded samples, got %d", got)
	}
	if r.active.samples[1].delta != 5 {
		t.Fatalf("second sample delta = %d, want 5 (the actual increase, not the request)", r.active.samples[1].delta)
	}
}
