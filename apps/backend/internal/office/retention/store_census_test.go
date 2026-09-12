package retention

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
)

func TestCensusRoutineRuns_RetainedCountIsSumOfEveryStatus(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))
	seedRoutineRun(t, conn, newID(), "r-1", "skipped", timePtr(daysAgo(2)), daysAgo(2))
	seedRoutineRun(t, conn, newID(), "r-1", "received", nil, daysAgo(0))

	census, err := store.CensusRoutineRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRoutineRuns: %v", err)
	}
	if census.RetainedCount != 3 {
		t.Fatalf("retainedCount = %d, want 3", census.RetainedCount)
	}
	if len(census.UnknownStatuses) != 0 {
		t.Fatalf("unknownStatuses = %v, want none", census.UnknownStatuses)
	}
}

func TestCensusRoutineRuns_EmptyTableReturnsZeroNoTopRoutine(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	census, err := store.CensusRoutineRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRoutineRuns: %v", err)
	}
	if census.RetainedCount != 0 {
		t.Fatalf("retainedCount = %d, want 0", census.RetainedCount)
	}
	if census.TopRoutineID != "" {
		t.Fatalf("topRoutineID = %q, want empty", census.TopRoutineID)
	}
}

func TestCensusRoutineRuns_DetectsUnknownStatus(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "quarantined", nil, daysAgo(1))
	seedRoutineRun(t, conn, newID(), "r-1", "quarantined", nil, daysAgo(2))

	census, err := store.CensusRoutineRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRoutineRuns: %v", err)
	}
	if len(census.UnknownStatuses) != 1 || census.UnknownStatuses[0].Status != "quarantined" || census.UnknownStatuses[0].Count != 2 {
		t.Fatalf("unknownStatuses = %v, want [{quarantined 2}]", census.UnknownStatuses)
	}
}

func TestCensusRoutineRuns_AttributesTopRoutineByRetainedShare(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	seedRoutine(t, conn, "r-heavy")
	seedRoutine(t, conn, "r-light")
	for i := 0; i < 3; i++ {
		seedRoutineRun(t, conn, newID(), "r-heavy", "done", timePtr(daysAgo(1)), daysAgo(1))
	}
	seedRoutineRun(t, conn, newID(), "r-light", "done", timePtr(daysAgo(1)), daysAgo(1))

	census, err := store.CensusRoutineRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRoutineRuns: %v", err)
	}
	if census.RetainedCount != 4 {
		t.Fatalf("retainedCount = %d, want 4", census.RetainedCount)
	}
	if census.TopRoutineID != "r-heavy" {
		t.Fatalf("topRoutineID = %q, want r-heavy", census.TopRoutineID)
	}
	if got, want := census.TopRoutineShare, 0.75; got != want {
		t.Fatalf("topRoutineShare = %v, want %v", got, want)
	}
}

func TestCensusRoutineRuns_TiesAttributeToLowerRoutineID(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	seedRoutine(t, conn, "r-b")
	seedRoutine(t, conn, "r-a")
	seedRoutineRun(t, conn, newID(), "r-b", "done", timePtr(daysAgo(1)), daysAgo(1))
	seedRoutineRun(t, conn, newID(), "r-a", "done", timePtr(daysAgo(1)), daysAgo(1))

	census, err := store.CensusRoutineRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRoutineRuns: %v", err)
	}
	if census.TopRoutineID != "r-a" {
		t.Fatalf("topRoutineID = %q, want r-a (lower id on tie)", census.TopRoutineID)
	}
}

func TestCensusRuns_RetainedCountIsSumOfEveryStatusNoTopAttribution(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	seedRun(t, conn, newID(), "agent-1", "finished", timePtr(daysAgo(1)), daysAgo(1))
	seedRun(t, conn, newID(), "agent-1", "queued", nil, daysAgo(0))
	seedRun(t, conn, newID(), "agent-1", "mystery", nil, daysAgo(0))
	seedRun(t, conn, newID(), "agent-1", "mystery", nil, daysAgo(0))

	census, err := store.CensusRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRuns: %v", err)
	}
	if census.RetainedCount != 4 {
		t.Fatalf("retainedCount = %d, want 4", census.RetainedCount)
	}
	if len(census.UnknownStatuses) != 1 || census.UnknownStatuses[0].Status != "mystery" || census.UnknownStatuses[0].Count != 2 {
		t.Fatalf("unknownStatuses = %v, want [{mystery 2}]", census.UnknownStatuses)
	}
	if census.TopRoutineID != "" {
		t.Fatalf("topRoutineID = %q, want empty (runs has no routine attribution)", census.TopRoutineID)
	}
}

func TestCensusRunEvents_PlainCountNoStatusDetection(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	runID := newID()
	seedRun(t, conn, runID, "agent-1", "finished", timePtr(daysAgo(1)), daysAgo(1))
	seedRunEvent(t, conn, runID, 1)
	seedRunEvent(t, conn, runID, 2)

	census, err := store.CensusRunEvents(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRunEvents: %v", err)
	}
	if census.RetainedCount != 2 {
		t.Fatalf("retainedCount = %d, want 2", census.RetainedCount)
	}
	if census.UnknownStatuses != nil {
		t.Fatalf("unknownStatuses = %v, want nil", census.UnknownStatuses)
	}
}

// TestCensusRoutineRuns_ConcurrentWriteBetweenUnknownStatusScanAndTotalsStaysConsistent
// proves the fix for the top-routine attribution's former two-query race: a
// write landing between the unknown-status scan and the single-statement
// totals read must not let the reported TopRoutineShare and RetainedCount
// come from different snapshots of the table. Before the fix, this seam sat
// between two independent reads and could make TopRoutineShare exceed 1.0
// or attribute a share against a stale total.
func TestCensusRoutineRuns_ConcurrentWriteBetweenUnknownStatusScanAndTotalsStaysConsistent(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	seedRoutine(t, conn, "r-1")
	seedRoutine(t, conn, "r-2")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))

	testBetweenRoutineRunCensusReads = func(queryer) {
		seedRoutineRun(t, conn, newID(), "r-2", "done", timePtr(daysAgo(1)), daysAgo(1))
		seedRoutineRun(t, conn, newID(), "r-2", "done", timePtr(daysAgo(1)), daysAgo(1))
	}
	t.Cleanup(func() { testBetweenRoutineRunCensusReads = nil })

	census, err := store.CensusRoutineRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRoutineRuns: %v", err)
	}

	if census.RetainedCount != 3 {
		t.Fatalf("retainedCount = %d, want 3 (the single totals read must see the concurrent write)", census.RetainedCount)
	}
	if census.TopRoutineID != "r-2" {
		t.Fatalf("topRoutineID = %q, want r-2", census.TopRoutineID)
	}
	if got, want := census.TopRoutineShare, 2.0/3.0; got != want {
		t.Fatalf("topRoutineShare = %v, want %v", got, want)
	}
	if census.TopRoutineShare > 1.0 {
		t.Fatalf("topRoutineShare = %v, must never exceed 1.0", census.TopRoutineShare)
	}
}

// TestCensusRoutineRuns_TableEmptiedBetweenReadsReturnsZeroWithoutError
// proves routineRunCensusTotals treats a table that became empty as the
// legitimate zero state rather than propagating sql.ErrNoRows: the old
// two-query design decided whether to run the top-routine query from a
// separately-read, now-stale nonzero total, so this same interleaving used
// to surface an unhandled error instead of a clean zero census.
func TestCensusRoutineRuns_TableEmptiedBetweenReadsReturnsZeroWithoutError(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))

	testBetweenRoutineRunCensusReads = func(queryer) {
		if _, err := conn.Exec(`DELETE FROM office_routine_runs`); err != nil {
			t.Fatalf("delete all rows: %v", err)
		}
	}
	t.Cleanup(func() { testBetweenRoutineRunCensusReads = nil })

	census, err := store.CensusRoutineRuns(ctx, conn, time.Now().UTC())
	if err != nil {
		t.Fatalf("CensusRoutineRuns: %v", err)
	}
	if census.RetainedCount != 0 {
		t.Fatalf("retainedCount = %d, want 0", census.RetainedCount)
	}
	if census.TopRoutineID != "" {
		t.Fatalf("topRoutineID = %q, want empty", census.TopRoutineID)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
