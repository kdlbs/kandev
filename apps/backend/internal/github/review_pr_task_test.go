package github

import (
	"context"
	"sync"
	"testing"
)

// recordingTaskDeleter records DeleteTask calls so tests can assert that
// the cleanup path with an empty task_id never invokes the deleter.
type recordingTaskDeleter struct {
	calls []string
}

func (r *recordingTaskDeleter) DeleteTask(_ context.Context, taskID string) error {
	r.calls = append(r.calls, taskID)
	return nil
}

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

// TestAssignReviewPRTaskID_ErrorsWhenNoReservation surfaces the narrow race
// where the dedup row was removed between Reserve and Assign (e.g. by a
// concurrent cleanup sweep). Silent no-op would leak the task with no dedup
// record, so we require an error.
func TestAssignReviewPRTaskID_ErrorsWhenNoReservation(t *testing.T) {
	_, store, _ := setupSyncTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	// No reservation was made — Assign must return an error, not silently no-op.
	err := store.AssignReviewPRTaskID(ctx, watch.ID, "acme", "widget", 42, "task-xyz")
	if err == nil {
		t.Fatal("expected error when no reservation exists, got nil")
	}
}

// TestCleanupMergedReviewTasks_OrphanReservation verifies that an orphan
// reservation row (empty task_id — process was killed between Reserve and
// Assign) is cleaned up when the PR reaches a terminal state, WITHOUT
// calling DeleteTask(""). The previous code relied on DeleteTask("")
// returning a "not found"-shaped error, which was fragile.
func TestCleanupMergedReviewTasks_OrphanReservation(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	// Simulate a crash between Reserve and Assign: a reservation row exists
	// with empty task_id.
	if _, err := store.ReserveReviewPRTask(
		ctx, watch.ID, "acme", "widget", 42,
		"https://github.com/acme/widget/pull/42",
	); err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	// PR is merged so cleanup is triggered.
	mockClient.AddPR(&PR{Number: 42, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})

	// Recording deleter: DeleteTask must NOT be called for the orphan row.
	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil {
		t.Fatalf("CleanupMergedReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
	if len(rec.calls) != 0 {
		t.Errorf("expected DeleteTask NOT to be called for orphan reservation, got calls=%v", rec.calls)
	}

	// The orphan dedup row must be gone.
	remaining, err := store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining rows, got %d", len(remaining))
	}
}

// TestCleanupMergedReviewTasks_OrphanReservationKeptWhilePROpen ensures the
// orphan row is retained while the PR is still open, so a subsequent poller
// tick can retry the task creation (by first releasing, then reserving again).
func TestCleanupMergedReviewTasks_OrphanReservationKeptWhilePROpen(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
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

	// PR is still open — cleanup should NOT remove this row.
	mockClient.AddPR(&PR{Number: 42, State: "open", RepoOwner: "acme", RepoName: "widget"})

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil {
		t.Fatalf("CleanupMergedReviewTasks: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted while PR open, got %d", deleted)
	}

	remaining, err := store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected 1 row retained while PR open, got %d", len(remaining))
	}
}
