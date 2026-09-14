package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func createTestRoutineForLastRun(t *testing.T, repo interface {
	CreateRoutine(ctx context.Context, routine *models.Routine) error
}) *models.Routine {
	t.Helper()
	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Last Run Test",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return routine
}

// AC-OFFICE-LOOP-LIVENESS-001.1/.3/.4/.6: TouchRoutineLastRun advances
// last_run_at from NULL, is monotonic against an equal or older instant
// (zero-row no-op, not an error), and moves forward for a newer instant.
func TestTouchRoutineLastRun_Monotonic(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := createTestRoutineForLastRun(t, repo)

	first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := repo.TouchRoutineLastRun(ctx, routine.ID, first); err != nil {
		t.Fatalf("touch (initial): %v", err)
	}
	got, err := repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(first) {
		t.Fatalf("last_run_at = %v, want %v", got.LastRunAt, first)
	}

	// Older instant: no-op.
	older := first.Add(-time.Hour)
	if err := repo.TouchRoutineLastRun(ctx, routine.ID, older); err != nil {
		t.Fatalf("touch (older): %v", err)
	}
	got, err = repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.LastRunAt.Equal(first) {
		t.Fatalf("last_run_at moved backwards: got %v, want %v", got.LastRunAt, first)
	}

	// Equal instant: no-op.
	if err := repo.TouchRoutineLastRun(ctx, routine.ID, first); err != nil {
		t.Fatalf("touch (equal): %v", err)
	}
	got, err = repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.LastRunAt.Equal(first) {
		t.Fatalf("last_run_at changed on equal instant: got %v, want %v", got.LastRunAt, first)
	}

	// Newer instant: advances.
	newer := first.Add(time.Hour)
	if err := repo.TouchRoutineLastRun(ctx, routine.ID, newer); err != nil {
		t.Fatalf("touch (newer): %v", err)
	}
	got, err = repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.LastRunAt.Equal(newer) {
		t.Fatalf("last_run_at = %v, want %v", got.LastRunAt, newer)
	}
}

// AC-OFFICE-LOOP-LIVENESS-001.6: a touch matching zero rows (unknown
// routine) is treated as success, not an error.
func TestTouchRoutineLastRun_UnknownRoutineIsNoop(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.TouchRoutineLastRun(ctx, "does-not-exist", time.Now().UTC()); err != nil {
		t.Fatalf("touch unknown routine: %v", err)
	}
}

// AC-OFFICE-LOOP-LIVENESS-001.5: the write touches only last_run_at (and
// updated_at) — every other column is left exactly as it was.
func TestTouchRoutineLastRun_TouchesOnlyLastRunAtAndUpdatedAt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := createTestRoutineForLastRun(t, repo)
	beforeUpdatedAt := routine.UpdatedAt

	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.TouchRoutineLastRun(ctx, routine.ID, at); err != nil {
		t.Fatalf("touch: %v", err)
	}

	got, err := repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != routine.Name || got.Status != routine.Status ||
		got.ConcurrencyPolicy != routine.ConcurrencyPolicy || got.TaskTemplate != routine.TaskTemplate {
		t.Fatalf("touch mutated a non-last_run_at column: %+v", got)
	}
	if !got.UpdatedAt.After(beforeUpdatedAt) {
		t.Fatalf("updated_at did not advance: got %v, before %v", got.UpdatedAt, beforeUpdatedAt)
	}
}

// AC-OFFICE-LOOP-LIVENESS-001.8: UpdateRoutine no longer carries
// last_run_at — a caller holding a stale (or nil) snapshot cannot move
// the fresh instant this dispatch just wrote backwards or clear it.
func TestUpdateRoutine_DoesNotWriteLastRunAt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := createTestRoutineForLastRun(t, repo)

	fresh := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.TouchRoutineLastRun(ctx, routine.ID, fresh); err != nil {
		t.Fatalf("touch: %v", err)
	}

	// A caller holding a stale snapshot (older LastRunAt) must not move
	// the stored instant backwards via an unrelated field update.
	stale := fresh.Add(-24 * time.Hour)
	routine.LastRunAt = &stale
	routine.Name = "Renamed While Stale"
	if err := repo.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("update (stale snapshot): %v", err)
	}
	got, err := repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(fresh) {
		t.Fatalf("stale UpdateRoutine moved last_run_at: got %v, want %v", got.LastRunAt, fresh)
	}
	if got.Name != "Renamed While Stale" {
		t.Fatalf("update did not apply other fields: name = %q", got.Name)
	}

	// A caller that never populated LastRunAt (nil) must not clear it either.
	routine.LastRunAt = nil
	routine.Description = "Updated from nil snapshot"
	if err := repo.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("update (nil snapshot): %v", err)
	}
	got, err = repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(fresh) {
		t.Fatalf("nil-snapshot UpdateRoutine cleared last_run_at: got %v, want %v", got.LastRunAt, fresh)
	}
	if got.Description != "Updated from nil snapshot" {
		t.Fatalf("update did not apply other fields: description = %q", got.Description)
	}

	// A caller holding a snapshot newer than the stored instant must not
	// advance it either: a stale-or-nil case alone cannot distinguish "the
	// column is not written" from "the column is written monotonically",
	// since both pass under a stale or nil snapshot. Only a newer snapshot
	// tells them apart.
	newer := fresh.Add(24 * time.Hour)
	routine.LastRunAt = &newer
	routine.Name = "Renamed From Newer Snapshot"
	if err := repo.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("update (newer snapshot): %v", err)
	}
	got, err = repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(fresh) {
		t.Fatalf("newer-snapshot UpdateRoutine advanced last_run_at: got %v, want %v", got.LastRunAt, fresh)
	}
	if got.Name != "Renamed From Newer Snapshot" {
		t.Fatalf("update did not apply other fields: name = %q", got.Name)
	}
}
