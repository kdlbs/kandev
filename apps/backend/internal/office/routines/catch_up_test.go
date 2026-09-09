package routines

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestComputeCatchUp_HappyPath: NextRunAt == now, no backlog — expect
// ElapsedTicks=1 (one fire due now), NextRunAt advances to the next cron
// tick, and no gap summary (FirstMissedAt zero, not truncated).
func TestComputeCatchUp_HappyPath(t *testing.T) {
	t0 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		Timezone:       "",
		NextRunAt:      &t0,
	}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 25}

	result := computeCatchUp(trigger, routine, t0)
	if result.Unknown {
		t.Fatalf("Unknown = true, want false")
	}
	if result.ElapsedTicks != 1 {
		t.Errorf("ElapsedTicks = %d, want 1", result.ElapsedTicks)
	}
	if !result.FirstMissedAt.IsZero() {
		t.Errorf("FirstMissedAt = %v, want zero (ElapsedTicks <= 1)", result.FirstMissedAt)
	}
	if result.Truncated {
		t.Error("Truncated = true, want false")
	}
	if !result.NextRunAt.Equal(t0.Add(time.Minute)) {
		t.Errorf("NextRunAt = %v, want %v", result.NextRunAt, t0.Add(time.Minute))
	}
}

// TestComputeCatchUp_BackendDownTenMinutes: NextRunAt is 10 minutes before
// now with an "every minute" cron — expect 11 elapsed ticks (10 missed + 1
// due now), FirstMissedAt exactly the armed next_run_at, not truncated.
func TestComputeCatchUp_BackendDownTenMinutes(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 10, 0, 0, time.UTC)
	tenAgo := now.Add(-10 * time.Minute)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		Timezone:       "",
		NextRunAt:      &tenAgo,
	}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 25}

	result := computeCatchUp(trigger, routine, now)
	if result.Unknown {
		t.Fatalf("Unknown = true, want false")
	}
	if result.ElapsedTicks != 11 {
		t.Errorf("ElapsedTicks = %d, want 11", result.ElapsedTicks)
	}
	if !result.FirstMissedAt.Equal(tenAgo) {
		t.Errorf("FirstMissedAt = %v, want %v (exact armed next_run_at)", result.FirstMissedAt, tenAgo)
	}
	if result.Truncated {
		t.Error("Truncated = true, want false")
	}
	if !result.NextRunAt.Equal(now.Add(time.Minute)) {
		t.Errorf("NextRunAt = %v, want %v", result.NextRunAt, now.Add(time.Minute))
	}
}

// TestComputeCatchUp_HitsCap: 100 minutes missed with cap 5 — expect
// ElapsedTicks=5 (capped), Truncated=true, FirstMissedAt still exact and
// un-truncated, NextRunAt strictly after now.
func TestComputeCatchUp_HitsCap(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	hundredAgo := now.Add(-100 * time.Minute)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		Timezone:       "",
		NextRunAt:      &hundredAgo,
	}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 5}

	result := computeCatchUp(trigger, routine, now)
	if result.Unknown {
		t.Fatalf("Unknown = true, want false")
	}
	if result.ElapsedTicks != 5 {
		t.Errorf("ElapsedTicks = %d, want 5 (capped)", result.ElapsedTicks)
	}
	if !result.Truncated {
		t.Error("Truncated = false, want true")
	}
	if !result.FirstMissedAt.Equal(hundredAgo) {
		t.Errorf("FirstMissedAt = %v, want %v (exact, even truncated)", result.FirstMissedAt, hundredAgo)
	}
	if !result.NextRunAt.After(now) {
		t.Errorf("NextRunAt = %v, want a time after %v", result.NextRunAt, now)
	}
}

// TestComputeCatchUp_DefaultCap: CatchUpMax of 0 clamps to the default
// (25) via models.NormaliseCatchUpMax. 100 minutes missed produces
// ElapsedTicks=25 (AC-001.4).
func TestComputeCatchUp_DefaultCap(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	hundredAgo := now.Add(-100 * time.Minute)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		NextRunAt:      &hundredAgo,
	}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 0}

	result := computeCatchUp(trigger, routine, now)
	if result.ElapsedTicks != models.CatchUpMaxDefault {
		t.Errorf("ElapsedTicks = %d, want %d (CatchUpMaxDefault)", result.ElapsedTicks, models.CatchUpMaxDefault)
	}
}

// TestComputeCatchUp_CeilingClamp: CatchUpMax of 5000 clamps to the
// ceiling (1000) via models.NormaliseCatchUpMax (AC-001.4). A gap of 2000
// minutes with an every-minute cron produces ElapsedTicks capped at 1000,
// not 2000.
func TestComputeCatchUp_CeilingClamp(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	longAgo := now.Add(-2000 * time.Minute)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		NextRunAt:      &longAgo,
	}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 5000}

	result := computeCatchUp(trigger, routine, now)
	if result.ElapsedTicks != models.CatchUpMaxCeiling {
		t.Errorf("ElapsedTicks = %d, want %d (CatchUpMaxCeiling)", result.ElapsedTicks, models.CatchUpMaxCeiling)
	}
	if !result.Truncated {
		t.Error("Truncated = false, want true")
	}
}

// TestComputeCatchUp_HourlyCron: NextRunAt is 5 hours before now with an
// hourly cron — expect 6 elapsed ticks (5 missed + 1 due).
func TestComputeCatchUp_HourlyCron(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	fiveAgo := now.Add(-5 * time.Hour)
	trigger := &RoutineTrigger{
		CronExpression: "0 * * * *", // top of every hour
		NextRunAt:      &fiveAgo,
	}
	routine := &Routine{CatchUpMax: 25}

	result := computeCatchUp(trigger, routine, now)
	if result.ElapsedTicks != 6 {
		t.Errorf("ElapsedTicks = %d, want 6", result.ElapsedTicks)
	}
	if !result.NextRunAt.Equal(now.Add(time.Hour)) {
		t.Errorf("NextRunAt = %v, want %v", result.NextRunAt, now.Add(time.Hour))
	}
}

// TestComputeCatchUp_ElapsedTicksOne_NoFirstMissedAt: exactly one elapsed
// tick means missedTicks == 0, so FirstMissedAt stays zero even though the
// walk succeeded — this is the boundary buildGapSummary relies on to
// record no gap summary.
func TestComputeCatchUp_ElapsedTicksOne_NoFirstMissedAt(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 30, 0, time.UTC)
	oneAgo := now.Add(-30 * time.Second)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		NextRunAt:      &oneAgo,
	}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 25}

	result := computeCatchUp(trigger, routine, now)
	if result.ElapsedTicks != 1 {
		t.Fatalf("ElapsedTicks = %d, want 1", result.ElapsedTicks)
	}
	if !result.FirstMissedAt.IsZero() {
		t.Errorf("FirstMissedAt = %v, want zero", result.FirstMissedAt)
	}
}

// TestComputeCatchUp_MalformedExpression: an unparseable cron expression
// drives Unknown=true and NextRunAt exactly 24 hours after the processing
// instant (AC-001.11) — the failure is deterministic in the stored
// (expression, timezone) pair, so the re-arm is the trigger's permanent
// dispatch cadence until it is deleted and recreated.
func TestComputeCatchUp_MalformedExpression(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	trigger := &RoutineTrigger{
		CronExpression: "not a cron expression",
		NextRunAt:      &past,
	}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 25}

	result := computeCatchUp(trigger, routine, now)
	if !result.Unknown {
		t.Fatal("Unknown = false, want true")
	}
	want := now.Add(catchUpFallbackInterval)
	if !result.NextRunAt.Equal(want) {
		t.Errorf("NextRunAt = %v, want %v (now + 24h)", result.NextRunAt, want)
	}
	// AC-001.11 requires the eventual warning to name "the underlying
	// error", which means it has to survive on catchUpResult rather than
	// being discarded at the point of failure.
	if result.Err == nil {
		t.Error("Err = nil, want the underlying NextCronTime parse error")
	}

	// A second tick a day later dispatches once more, not repeatedly: the
	// re-armed NextRunAt is itself now due, and re-computing from it
	// produces the same 24h fallback rather than a hot loop.
	again := computeCatchUp(trigger, routine, result.NextRunAt)
	if !again.Unknown {
		t.Fatal("second computeCatchUp: Unknown = false, want true")
	}
	if !again.NextRunAt.Equal(result.NextRunAt.Add(catchUpFallbackInterval)) {
		t.Errorf("second NextRunAt = %v, want %v", again.NextRunAt, result.NextRunAt.Add(catchUpFallbackInterval))
	}
}

// TestBuildGapSummary_SkipMissedRecordsNothing: skip_missed never records
// a gap summary, whatever the elapsed-tick count (AC-002.7).
func TestBuildGapSummary_SkipMissedRecordsNothing(t *testing.T) {
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySkipMissed}
	result := catchUpResult{ElapsedTicks: 10, FirstMissedAt: time.Now(), Truncated: true}
	if gap := buildGapSummary(routine, result); gap != nil {
		t.Errorf("gap = %+v, want nil", gap)
	}
}

// TestBuildGapSummary_UnknownRecordsNothing: a failed walk never records a
// gap, even though ElapsedTicks is technically non-meaningful (AC-001.6).
func TestBuildGapSummary_UnknownRecordsNothing(t *testing.T) {
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed}
	result := catchUpResult{Unknown: true}
	if gap := buildGapSummary(routine, result); gap != nil {
		t.Errorf("gap = %+v, want nil", gap)
	}
}

// TestBuildGapSummary_ZeroMissedRecordsNothing: exactly one elapsed tick
// (missedTicks == 0) records no gap summary, whatever truncated would say
// (AC-002.2).
func TestBuildGapSummary_ZeroMissedRecordsNothing(t *testing.T) {
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed}
	result := catchUpResult{ElapsedTicks: 1}
	if gap := buildGapSummary(routine, result); gap != nil {
		t.Errorf("gap = %+v, want nil", gap)
	}
}

// TestBuildGapSummary_CatchUpMaxOne: catch_up_max == 1 makes missedTicks
// structurally zero regardless of the gap's real size — an opt-out of gap
// reporting, not an edge case (AC-002.11).
func TestBuildGapSummary_CatchUpMaxOne(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	longAgo := now.Add(-100 * time.Hour)
	trigger := &RoutineTrigger{CronExpression: "* * * * *", NextRunAt: &longAgo}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 1}

	// Truncation and a zero missed-tick count can co-occur only at exactly
	// catch_up_max == 1 (above 1 a truncated count is always >= cap - 1);
	// what matters here is that no gap summary is ever produced, whatever
	// Truncated says.
	result := computeCatchUp(trigger, routine, now)
	if gap := buildGapSummary(routine, result); gap != nil {
		t.Errorf("gap = %+v, want nil (catch_up_max == 1 opts out)", gap)
	}
}

// TestBuildGapSummary_SummarizeMissedRecordsGap: the happy path — a
// multi-tick gap under summarize_missed records the count, the exact
// first-missed timestamp, and truncated.
func TestBuildGapSummary_SummarizeMissedRecordsGap(t *testing.T) {
	first := time.Date(2026, 5, 10, 11, 55, 0, 0, time.UTC)
	result := catchUpResult{ElapsedTicks: 6, FirstMissedAt: first, Truncated: true}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicySummarizeMissed}

	gap := buildGapSummary(routine, result)
	if gap == nil {
		t.Fatal("gap = nil, want non-nil")
	}
	if gap.MissedTicks != 5 {
		t.Errorf("MissedTicks = %d, want 5", gap.MissedTicks)
	}
	if !gap.FirstMissed.Equal(first) {
		t.Errorf("FirstMissed = %v, want %v", gap.FirstMissed, first)
	}
	if !gap.Truncated {
		t.Error("Truncated = false, want true")
	}
}

// TestBuildGapSummary_DeprecatedAliasStillSummarizes proves the alias
// normalizes to summarize_missed's behaviour rather than being treated as
// an unrecognized (and thus also summarizing, per models.NormaliseCatchUpPolicy)
// value by accident — the policy comparison in buildGapSummary only ever
// special-cases skip_missed, so any non-skip value, including the raw
// alias constant, produces a gap summary.
func TestBuildGapSummary_DeprecatedAliasStillSummarizes(t *testing.T) {
	result := catchUpResult{ElapsedTicks: 3, FirstMissedAt: time.Now()}
	routine := &Routine{CatchUpPolicy: models.CatchUpPolicyEnqueueMissedWithCap}

	gap := buildGapSummary(routine, result)
	if gap == nil {
		t.Fatal("gap = nil, want non-nil")
	}
	if gap.MissedTicks != 2 {
		t.Errorf("MissedTicks = %d, want 2", gap.MissedTicks)
	}
}
