package coordinator

import (
	"context"
	"testing"
	"time"
)

// TestUpsertStall_Postgres_FenceOnStrictlyNewerLastEventAt is the PostgreSQL
// twin of the stall upsert fence tests: needs-you.md#stall-records requires
// the same INSERT ... ON CONFLICT ... WHERE statement to behave identically
// on both dialects.
func TestUpsertStall_Postgres_FenceOnStrictlyNewerLastEventAt(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	first := time.Now().UTC()

	matched, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", first))
	if err != nil {
		t.Fatalf("UpsertStall (first): %v", err)
	}
	if !matched {
		t.Fatal("UpsertStall did not report a new row as affected")
	}

	redelivered := sampleStall("task-1", "ws-1", first)
	redelivered.StalledForMs = 999_000
	matched, err = store.UpsertStall(ctx, redelivered)
	if err != nil {
		t.Fatalf("UpsertStall (equal): %v", err)
	}
	if matched {
		t.Fatal("UpsertStall reported a redelivery (equal last_event_at) as affected")
	}

	newer := sampleStall("task-1", "ws-1", first.Add(time.Minute))
	newer.StalledForMs = 120_000
	matched, err = store.UpsertStall(ctx, newer)
	if err != nil {
		t.Fatalf("UpsertStall (newer): %v", err)
	}
	if !matched {
		t.Fatal("UpsertStall did not report the newer episode as affected")
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListStalls returned %d rows, want 1", len(list))
	}
	if list[0].StalledForMs != 120_000 {
		t.Fatalf("ListStalls[0].StalledForMs = %d, want 120000", list[0].StalledForMs)
	}
}
