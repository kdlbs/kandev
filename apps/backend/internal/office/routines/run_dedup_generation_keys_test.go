package routines

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
)

// AC-OFFICE-RUN-DEDUP-001.1 / .4: a cron fire's key names the claimed
// scheduled slot, and redelivering the same slot (the same claimedTick)
// reproduces the same key byte for byte.
func TestBuildRoutineIdempotencyKey_Cron_UsesClaimedTick(t *testing.T) {
	tick := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	key := buildRoutineIdempotencyKey(shared.RoutineSourceCron, "routine-1", "trigger-1", &tick, "run-ignored")

	want := fmt.Sprintf("routine:%s:%s:tick:%d", "routine-1", "trigger-1", tick.Unix())
	if key != want {
		t.Fatalf("key = %q, want %q", key, want)
	}

	// A second claim of the identical slot (redelivery) mints the same key.
	repeat := buildRoutineIdempotencyKey(shared.RoutineSourceCron, "routine-1", "trigger-1", &tick, "run-different")
	if repeat != key {
		t.Fatalf("redelivery of the same slot produced %q, want %q (identical to the first)", repeat, key)
	}
}

// AC-OFFICE-RUN-DEDUP-001.2: two distinct cron slots (a genuinely later
// tick) mint two distinct keys.
func TestBuildRoutineIdempotencyKey_Cron_DistinctTicksDiffer(t *testing.T) {
	tickOne := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	tickTwo := tickOne.Add(time.Minute)

	keyOne := buildRoutineIdempotencyKey(shared.RoutineSourceCron, "routine-1", "trigger-1", &tickOne, "run-a")
	keyTwo := buildRoutineIdempotencyKey(shared.RoutineSourceCron, "routine-1", "trigger-1", &tickTwo, "run-b")

	if keyOne == keyTwo {
		t.Fatalf("distinct cron slots must mint distinct keys, both got %q", keyOne)
	}
}

// AC-OFFICE-RUN-DEDUP-003.3: a cron fire with no claimed tick (the
// exported entry point called directly with a nil tick, bypassing the
// live processCronTrigger guard) has no occurrence identity and goes
// keyless with cause=unresolved.
func TestBuildRoutineIdempotencyKey_Cron_NoClaimedTickGoesKeyless(t *testing.T) {
	key := buildRoutineIdempotencyKey(shared.RoutineSourceCron, "routine-1", "trigger-1", nil, "run-1")
	if key != "" {
		t.Fatalf("key = %q, want empty (keyless) for a cron fire with no claimed tick", key)
	}
}

// Manual and webhook fires claim no slot, so two fires of one routine are
// two distinct occurrences by design: each RoutineRun.ID mints its own key.
func TestBuildRoutineIdempotencyKey_ManualAndWebhook_UseRoutineRunID(t *testing.T) {
	for _, source := range []string{"manual", "webhook"} {
		key := buildRoutineIdempotencyKey(source, "routine-1", "", nil, "run-1")
		want := "routine:routine-1:run:run-1"
		if key != want {
			t.Errorf("source=%q key = %q, want %q", source, key, want)
		}
	}

	// Two distinct manual fires -> two distinct keys (the collision the
	// old unix-minute key format could reach).
	first := buildRoutineIdempotencyKey("manual", "routine-1", "", nil, "run-a")
	second := buildRoutineIdempotencyKey("manual", "routine-1", "", nil, "run-b")
	if first == second {
		t.Fatalf("two distinct manual fires must mint distinct keys, both got %q", first)
	}
}

// An unrecognised RoutineRun.Source names no occurrence this table covers
// and goes keyless with cause=unresolved, the same direction as a cron
// fire with no claimed tick.
func TestBuildRoutineIdempotencyKey_UnrecognisedSourceGoesKeyless(t *testing.T) {
	key := buildRoutineIdempotencyKey("some_future_source", "routine-1", "", nil, "run-1")
	if key != "" {
		t.Fatalf("key = %q, want empty (keyless) for an unrecognised source", key)
	}
}
