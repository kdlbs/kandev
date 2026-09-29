package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"
)

func sampleStall(taskID, workspaceID string, lastEventAt time.Time) *Stall {
	return &Stall{
		TaskID:       taskID,
		WorkspaceID:  workspaceID,
		StalledForMs: 60_000,
		LastEventAt:  lastEventAt,
		DetectedAt:   time.Now().UTC(),
	}
}

func TestUpsertStall_InsertsNewRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	stall := sampleStall("task-1", "ws-1", time.Now().UTC())

	matched, err := store.UpsertStall(ctx, stall)
	if err != nil {
		t.Fatalf("UpsertStall: %v", err)
	}
	if !matched {
		t.Fatal("UpsertStall did not report a new row as affected")
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListStalls returned %d rows, want 1", len(list))
	}
	got := list[0]
	if got.TaskID != "task-1" || got.WorkspaceID != "ws-1" || got.StalledForMs != 60_000 {
		t.Fatalf("ListStalls[0] = %+v", got)
	}
}

func TestUpsertStall_ReplacesOnStrictlyNewerLastEventAt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := time.Now().UTC()

	if _, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", first)); err != nil {
		t.Fatalf("UpsertStall (first): %v", err)
	}

	newer := sampleStall("task-1", "ws-1", first.Add(time.Minute))
	newer.StalledForMs = 120_000
	matched, err := store.UpsertStall(ctx, newer)
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
	if list[0].StalledForMs != 120_000 || !list[0].LastEventAt.Equal(newer.LastEventAt) {
		t.Fatalf("ListStalls[0] = %+v, want the newer episode", list[0])
	}
}

func TestUpsertStall_IgnoresEqualOrEarlierLastEventAt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first := time.Now().UTC()

	if _, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", first)); err != nil {
		t.Fatalf("UpsertStall (first): %v", err)
	}

	// Equal last_event_at: a redelivery, must change nothing.
	redelivered := sampleStall("task-1", "ws-1", first)
	redelivered.StalledForMs = 999_000
	matched, err := store.UpsertStall(ctx, redelivered)
	if err != nil {
		t.Fatalf("UpsertStall (equal): %v", err)
	}
	if matched {
		t.Fatal("UpsertStall reported a redelivery (equal last_event_at) as affected")
	}

	// Earlier last_event_at: an out-of-order event, must change nothing.
	earlier := sampleStall("task-1", "ws-1", first.Add(-time.Minute))
	earlier.StalledForMs = 1_000
	matched, err = store.UpsertStall(ctx, earlier)
	if err != nil {
		t.Fatalf("UpsertStall (earlier): %v", err)
	}
	if matched {
		t.Fatal("UpsertStall reported an out-of-order event as affected")
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListStalls returned %d rows, want 1", len(list))
	}
	if list[0].StalledForMs != 60_000 || !list[0].LastEventAt.Equal(first) {
		t.Fatalf("ListStalls[0] = %+v, want the original episode unchanged", list[0])
	}
}

func TestDeleteStall_RemovesRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", time.Now().UTC())); err != nil {
		t.Fatalf("UpsertStall: %v", err)
	}

	if err := store.DeleteStall(ctx, "task-1"); err != nil {
		t.Fatalf("DeleteStall: %v", err)
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListStalls after delete returned %d rows, want 0", len(list))
	}
}

func TestDeleteStall_UnknownOrRepeatedIsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.DeleteStall(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteStall(missing): err = %v, want ErrNotFound", err)
	}

	if _, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", time.Now().UTC())); err != nil {
		t.Fatalf("UpsertStall: %v", err)
	}
	if err := store.DeleteStall(ctx, "task-1"); err != nil {
		t.Fatalf("first DeleteStall: %v", err)
	}
	if err := store.DeleteStall(ctx, "task-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeated DeleteStall: err = %v, want ErrNotFound", err)
	}
}

func TestListStalls_OrderedByTaskIDScopedToWorkspace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := store.UpsertStall(ctx, sampleStall("task-b", "ws-1", now)); err != nil {
		t.Fatalf("UpsertStall task-b: %v", err)
	}
	if _, err := store.UpsertStall(ctx, sampleStall("task-a", "ws-1", now)); err != nil {
		t.Fatalf("UpsertStall task-a: %v", err)
	}
	if _, err := store.UpsertStall(ctx, sampleStall("task-c", "ws-2", now)); err != nil {
		t.Fatalf("UpsertStall task-c (other workspace): %v", err)
	}

	list, err := store.ListStalls(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListStalls returned %d rows, want 2", len(list))
	}
	if list[0].TaskID != "task-a" || list[1].TaskID != "task-b" {
		t.Fatalf("ListStalls order = [%s, %s], want [task-a, task-b]", list[0].TaskID, list[1].TaskID)
	}
}

func TestListStalls_EmptyIsEmptySliceNotNil(t *testing.T) {
	store := newTestStore(t)
	list, err := store.ListStalls(context.Background(), "ws-none")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if list == nil {
		t.Fatal("ListStalls returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Fatalf("ListStalls returned %d rows, want 0", len(list))
	}
}

func TestGetStall_ReturnsRowScopedToWorkspace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", now)); err != nil {
		t.Fatalf("UpsertStall: %v", err)
	}

	got, err := store.GetStall(ctx, "ws-1", "task-1")
	if err != nil {
		t.Fatalf("GetStall: %v", err)
	}
	if got.TaskID != "task-1" || got.WorkspaceID != "ws-1" || got.StalledForMs != 60_000 {
		t.Fatalf("GetStall = %+v", got)
	}
}

func TestGetStall_WrongWorkspaceIsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.UpsertStall(ctx, sampleStall("task-1", "ws-1", time.Now().UTC())); err != nil {
		t.Fatalf("UpsertStall: %v", err)
	}

	if _, err := store.GetStall(ctx, "ws-2", "task-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetStall(wrong workspace): err = %v, want ErrNotFound", err)
	}
}

func TestGetStall_MissingTaskIsNotFound(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.GetStall(context.Background(), "ws-1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetStall(missing): err = %v, want ErrNotFound", err)
	}
}
