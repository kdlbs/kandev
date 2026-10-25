package retention

import (
	"context"
	"testing"
	"time"
)

func TestPreviewMarkerStore_MissingIsReadableAndEmpty(t *testing.T) {
	_, raw := newTestSettingsStore(t)
	store := NewPreviewMarkerStore(raw)

	marker, readable := store.Get(context.Background())
	if !readable {
		t.Fatalf("Get() readable = false, want true for a never-written marker")
	}
	if len(marker) != 0 {
		t.Fatalf("Get() marker = %+v, want empty", marker)
	}
}

func TestPreviewMarkerStore_MarkCompletedThenGetRoundTrips(t *testing.T) {
	_, raw := newTestSettingsStore(t)
	store := NewPreviewMarkerStore(raw)
	ctx := context.Background()

	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if err := store.MarkCompleted(ctx, TableOfficeRoutineRuns, at); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	marker, readable := store.Get(ctx)
	if !readable {
		t.Fatalf("Get() readable = false after a valid write")
	}
	got, ok := marker[TableOfficeRoutineRuns]
	if !ok {
		t.Fatalf("marker missing office_routine_runs entry: %+v", marker)
	}
	if !got.Equal(at) {
		t.Fatalf("marker[office_routine_runs] = %v, want %v", got, at)
	}
	if _, ok := marker[TableRuns]; ok {
		t.Fatalf("marker has an entry for runs before it was ever marked: %+v", marker)
	}
}

// TestPreviewMarkerStore_PerTableNotGlobal proves the marker is per swept
// table, not per database: marking one table previewed must not mark a
// sibling table previewed too (AC-OFFICE-RUN-HISTORY-RETENTION-003.4).
func TestPreviewMarkerStore_PerTableNotGlobal(t *testing.T) {
	_, raw := newTestSettingsStore(t)
	store := NewPreviewMarkerStore(raw)
	ctx := context.Background()

	if err := store.MarkCompleted(ctx, TableOfficeRoutineRuns, time.Now().UTC()); err != nil {
		t.Fatalf("MarkCompleted(office_routine_runs): %v", err)
	}

	marker, _ := store.Get(ctx)
	if _, ok := marker[TableRuns]; ok {
		t.Fatalf("marking office_routine_runs previewed also marked runs: %+v", marker)
	}

	if err := store.MarkCompleted(ctx, TableRuns, time.Now().UTC()); err != nil {
		t.Fatalf("MarkCompleted(runs): %v", err)
	}
	marker, _ = store.Get(ctx)
	if len(marker) != 2 {
		t.Fatalf("marker after both tables previewed = %+v, want 2 entries", marker)
	}
}

// TestPreviewMarkerStore_UnparseableTreatsEveryTableAsNotPreviewed proves
// AC-OFFICE-RUN-HISTORY-RETENTION-003.10: a present-but-corrupt marker
// document is treated as "not yet previewed" for every swept table, which
// is the safe direction because a spurious re-preview deletes nothing.
func TestPreviewMarkerStore_UnparseableTreatsEveryTableAsNotPreviewed(t *testing.T) {
	_, raw := newTestSettingsStore(t)
	ctx := context.Background()
	if err := raw.Save(ctx, previewMarkerKey, []byte("not json")); err != nil {
		t.Fatalf("seed unparseable marker: %v", err)
	}

	store := NewPreviewMarkerStore(raw)
	marker, readable := store.Get(ctx)
	if readable {
		t.Fatalf("Get() readable = true for an unparseable marker, want false")
	}
	if len(marker) != 0 {
		t.Fatalf("Get() marker = %+v on unparseable document, want empty (not-previewed)", marker)
	}
}

// TestPreviewMarkerStore_MarkCompletedRecoversFromUnparseable proves that
// marking a table previewed after a corrupt document is detected replaces
// the corrupt document rather than erroring forever.
func TestPreviewMarkerStore_MarkCompletedRecoversFromUnparseable(t *testing.T) {
	_, raw := newTestSettingsStore(t)
	ctx := context.Background()
	if err := raw.Save(ctx, previewMarkerKey, []byte("not json")); err != nil {
		t.Fatalf("seed unparseable marker: %v", err)
	}
	store := NewPreviewMarkerStore(raw)

	at := time.Now().UTC()
	if err := store.MarkCompleted(ctx, TableRuns, at); err != nil {
		t.Fatalf("MarkCompleted after corrupt document: %v", err)
	}
	marker, readable := store.Get(ctx)
	if !readable {
		t.Fatalf("Get() readable = false after a fresh valid write")
	}
	if _, ok := marker[TableRuns]; !ok {
		t.Fatalf("marker missing runs entry after recovery write: %+v", marker)
	}
}
