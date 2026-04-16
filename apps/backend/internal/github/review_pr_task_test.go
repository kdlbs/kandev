package github

import (
	"context"
	"sync"
	"testing"
)

// TestReserveReviewPRTask_AtomicUnderConcurrency verifies that when N goroutines
// race to reserve the same (watch_id, repo, pr_number), exactly one wins.
// This is the core invariant that prevents duplicate review tasks from being
// created: the reservation must be atomic BEFORE the slow task-creation work.
func TestReserveReviewPRTask_AtomicUnderConcurrency(t *testing.T) {
	_, store, _ := setupSyncTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	const goroutines = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	var wins int
	var errs []error
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			reserved, err := store.ReserveReviewPRTask(
				ctx, watch.ID, "acme", "widget", 42,
				"https://github.com/acme/widget/pull/42",
			)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if reserved {
				wins++
			}
		}()
	}
	wg.Wait()

	if len(errs) != 0 {
		t.Fatalf("unexpected errors from concurrent Reserve: %v", errs)
	}
	if wins != 1 {
		t.Errorf("expected exactly 1 reservation to win, got %d", wins)
	}

	// The dedup row must exist exactly once.
	records, err := store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 dedup record, got %d", len(records))
	}
}

// TestReserveReviewPRTask_ReturnsFalseWhenAlreadyReserved verifies that a
// second reservation for the same PR returns (false, nil) rather than an
// error, so callers can treat it as a signal to bail out.
func TestReserveReviewPRTask_ReturnsFalseWhenAlreadyReserved(t *testing.T) {
	_, store, _ := setupSyncTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	first, err := store.ReserveReviewPRTask(
		ctx, watch.ID, "acme", "widget", 42,
		"https://github.com/acme/widget/pull/42",
	)
	if err != nil {
		t.Fatalf("first Reserve: %v", err)
	}
	if !first {
		t.Fatal("first reservation must succeed")
	}

	second, err := store.ReserveReviewPRTask(
		ctx, watch.ID, "acme", "widget", 42,
		"https://github.com/acme/widget/pull/42",
	)
	if err != nil {
		t.Fatalf("second Reserve returned error (want (false, nil)): %v", err)
	}
	if second {
		t.Error("second reservation must return false")
	}
}

// TestReleaseReviewPRTask_RemovesReservation verifies that releasing a
// reservation makes the slot available again (e.g. when task creation fails
// and we want a subsequent poller tick to retry).
func TestReleaseReviewPRTask_RemovesReservation(t *testing.T) {
	_, store, _ := setupSyncTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	if _, err := store.ReserveReviewPRTask(
		ctx, watch.ID, "acme", "widget", 42,
		"https://github.com/acme/widget/pull/42",
	); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	if err := store.ReleaseReviewPRTask(ctx, watch.ID, "acme", "widget", 42); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// After release, a new reservation must succeed again.
	reserved, err := store.ReserveReviewPRTask(
		ctx, watch.ID, "acme", "widget", 42,
		"https://github.com/acme/widget/pull/42",
	)
	if err != nil {
		t.Fatalf("second Reserve: %v", err)
	}
	if !reserved {
		t.Error("expected reservation to succeed after release")
	}
}

// TestAssignReviewPRTaskID_UpdatesTaskID verifies that the task_id of a
// reserved (and thus initially empty-task_id) dedup row is updated so
// downstream cleanup logic can find and delete the task.
func TestAssignReviewPRTaskID_UpdatesTaskID(t *testing.T) {
	_, store, _ := setupSyncTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	if _, err := store.ReserveReviewPRTask(
		ctx, watch.ID, "acme", "widget", 42,
		"https://github.com/acme/widget/pull/42",
	); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	if err := store.AssignReviewPRTaskID(ctx, watch.ID, "acme", "widget", 42, "task-xyz"); err != nil {
		t.Fatalf("AssignReviewPRTaskID: %v", err)
	}

	records, err := store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].TaskID != "task-xyz" {
		t.Errorf("TaskID = %q, want %q", records[0].TaskID, "task-xyz")
	}
}
