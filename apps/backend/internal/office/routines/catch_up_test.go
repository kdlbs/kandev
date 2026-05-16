package routines

import (
	"testing"
	"time"
)

// TestComputeRoutineMissed_HappyPath: NextRunAt == now, no backlog —
// expect runCount=1 (one fire due now), advance to next cron tick.
func TestComputeRoutineMissed_HappyPath(t *testing.T) {
	t0 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		Timezone:       "",
		NextRunAt:      &t0,
	}
	routine := &Routine{CatchUpPolicy: CatchUpPolicyEnqueueWithCap, CatchUpMax: 25}

	count, advanceTo, err := computeRoutineMissed(trigger, routine, t0)
	if err != nil {
		t.Fatalf("computeRoutineMissed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if !advanceTo.Equal(t0.Add(time.Minute)) {
		t.Errorf("advanceTo = %v, want %v", advanceTo, t0.Add(time.Minute))
	}
}

// TestComputeRoutineMissed_BackendDownTenMinutes: NextRunAt is 10 minutes
// before now with an "every minute" cron — expect 11 ticks counted (10
// missed + 1 due now), advance to the next-minute tick.
func TestComputeRoutineMissed_BackendDownTenMinutes(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 10, 0, 0, time.UTC)
	tenAgo := now.Add(-10 * time.Minute)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		Timezone:       "",
		NextRunAt:      &tenAgo,
	}
	routine := &Routine{CatchUpPolicy: CatchUpPolicyEnqueueWithCap, CatchUpMax: 25}

	count, advanceTo, err := computeRoutineMissed(trigger, routine, now)
	if err != nil {
		t.Fatalf("computeRoutineMissed: %v", err)
	}
	if count != 11 {
		t.Errorf("count = %d, want 11", count)
	}
	if !advanceTo.Equal(now.Add(time.Minute)) {
		t.Errorf("advanceTo = %v, want %v", advanceTo, now.Add(time.Minute))
	}
}

// TestComputeRoutineMissed_HitsCap: 100 minutes missed with cap 5 —
// expect runCount=5 (capped), advanceTo = next future tick from now so
// the trigger leaves the catch-up window cleanly.
func TestComputeRoutineMissed_HitsCap(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	hundredAgo := now.Add(-100 * time.Minute)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		Timezone:       "",
		NextRunAt:      &hundredAgo,
	}
	routine := &Routine{CatchUpPolicy: CatchUpPolicyEnqueueWithCap, CatchUpMax: 5}

	count, advanceTo, err := computeRoutineMissed(trigger, routine, now)
	if err != nil {
		t.Fatalf("computeRoutineMissed: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5 (capped)", count)
	}
	if !advanceTo.After(now) {
		t.Errorf("advanceTo = %v, want a time after %v", advanceTo, now)
	}
}

// TestComputeRoutineMissed_DefaultCap: CatchUpMax of 0 falls back to
// the default (25). 100 minutes missed produces runCount=25.
func TestComputeRoutineMissed_DefaultCap(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	hundredAgo := now.Add(-100 * time.Minute)
	trigger := &RoutineTrigger{
		CronExpression: "* * * * *",
		NextRunAt:      &hundredAgo,
	}
	// CatchUpMax=0 → defaultCatchUpMax (25).
	routine := &Routine{CatchUpPolicy: CatchUpPolicyEnqueueWithCap, CatchUpMax: 0}

	count, _, err := computeRoutineMissed(trigger, routine, now)
	if err != nil {
		t.Fatalf("computeRoutineMissed: %v", err)
	}
	if count != defaultCatchUpMax {
		t.Errorf("count = %d, want %d (defaultCatchUpMax)", count, defaultCatchUpMax)
	}
}

// TestComputeRoutineMissed_HourlyCron: NextRunAt is 5 hours before now
// with an hourly cron — expect 6 ticks counted (5 missed + 1 due).
func TestComputeRoutineMissed_HourlyCron(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	fiveAgo := now.Add(-5 * time.Hour)
	trigger := &RoutineTrigger{
		CronExpression: "0 * * * *", // top of every hour
		NextRunAt:      &fiveAgo,
	}
	routine := &Routine{CatchUpMax: 25}

	count, advanceTo, err := computeRoutineMissed(trigger, routine, now)
	if err != nil {
		t.Fatalf("computeRoutineMissed: %v", err)
	}
	if count != 6 {
		t.Errorf("count = %d, want 6", count)
	}
	if !advanceTo.Equal(now.Add(time.Hour)) {
		t.Errorf("advanceTo = %v, want %v", advanceTo, now.Add(time.Hour))
	}
}
