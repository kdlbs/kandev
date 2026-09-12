package routines_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// AC-OFFICE-RUN-DEDUP-001.9: the cron dispatch path threads the trigger's
// claimed tick into the wakeup-request's idempotency key rather than
// re-reading trigger.next_run_at after TickScheduledTriggers has already
// advanced it to the next slot. Driving this through TickScheduledTriggers
// (not the private key builder) is what actually pins the wiring: a future
// change that re-reads the trigger row after the advance would still pass
// the unit-level key-format tests but fail here.
func TestTickScheduledTriggers_WakeupKeyNamesClaimedTick_NotAdvancedRow(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)
	trigger := &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	if err := svc.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: %v (n=%d)", err, len(triggers))
	}
	claimedTick := *triggers[0].NextRunAt

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if err := svc.TickScheduledTriggers(ctx, claimedTick.Add(2*time.Minute)); err != nil {
		t.Fatalf("tick scheduled triggers: %v", err)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup-request created, got %d", len(enq.created))
	}

	want := fmt.Sprintf("routine:%s:%s:tick:%d", routine.ID, triggers[0].ID, claimedTick.Unix())
	got := enq.created[0].IdempotencyKey
	if got != want {
		t.Fatalf("idempotency key = %q, want %q (the claimed tick, not the row's advanced next_run_at)", got, want)
	}
}

// AC-OFFICE-RUN-DEDUP-001.2: two manual "Fire now" calls inside the same
// minute are two distinct occurrences (an operator asking twice) and must
// mint two distinct keys - the exact collision the old unix-minute key
// format reached (both fires share triggerID == "" and can share a minute).
func TestFireManual_TwoFiresInSameMinute_MintDistinctKeys(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)
	routine.ConcurrencyPolicy = "always_create"
	if err := svc.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("update routine: %v", err)
	}

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if _, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "one"}); err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if _, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "two"}); err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if len(enq.created) != 2 {
		t.Fatalf("expected 2 wakeup-requests created, got %d", len(enq.created))
	}
	first, second := enq.created[0].IdempotencyKey, enq.created[1].IdempotencyKey
	if first == "" || second == "" {
		t.Fatalf("expected non-empty keys, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("two distinct manual fires must mint distinct keys, both got %q", first)
	}
}
