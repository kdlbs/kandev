package github

import (
	"context"
	"testing"
	"time"
)

// TestUpdateTaskPR_NeverTouchesDispositionColumns pins the disjoint-writer
// guarantee from the spec's Concurrency section: UpdateTaskPR is the sync
// writer's exclusive write path and must never alter any disposition*
// column, regardless of what the in-memory TaskPR struct carries. A poll
// landing between a user's read and their disposition PATCH must not
// clobber the disposition (AC-29c).
func TestUpdateTaskPR_NeverTouchesDispositionColumns(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedDispositionTestTask(t, store, "task-disjoint-sync", "ws-disjoint-sync")

	now := time.Now().UTC()
	disposition := TaskPRDispositionSuperseded
	supersededURL := "https://github.com/kdlbs/kandev/pull/9999"
	tp := &TaskPR{
		TaskID:                     "task-disjoint-sync",
		Owner:                      "kdlbs",
		Repo:                       "kandev",
		PRNumber:                   1,
		PRURL:                      "https://github.com/kdlbs/kandev/pull/1",
		PRTitle:                    "disjoint writer test",
		State:                      "closed",
		CreatedAt:                  now,
		Disposition:                &disposition,
		DispositionSupersededByURL: &supersededURL,
		DispositionRecordedAt:      &now,
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}

	// Simulate a sync write racing in with a struct that has the disposition
	// fields cleared in memory. If UpdateTaskPR's SQL ever referenced those
	// fields, this would null out the recorded disposition.
	syncWrite := *tp
	syncWrite.Disposition = nil
	syncWrite.DispositionSupersededByURL = nil
	syncWrite.DispositionRecordedAt = nil
	syncWrite.State = "merged"
	mergedAt := now.Add(time.Minute)
	syncWrite.MergedAt = &mergedAt
	if err := store.UpdateTaskPR(ctx, &syncWrite); err != nil {
		t.Fatalf("UpdateTaskPR: %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.State != "merged" {
		t.Fatalf("State = %q, want %q (UpdateTaskPR should have applied the sync fields)", got.State, "merged")
	}
	if got.Disposition == nil || *got.Disposition != TaskPRDispositionSuperseded {
		t.Fatalf("Disposition = %v, want unchanged %q (UpdateTaskPR must never touch disposition columns)", got.Disposition, TaskPRDispositionSuperseded)
	}
	if got.DispositionSupersededByURL == nil || *got.DispositionSupersededByURL != supersededURL {
		t.Fatalf("DispositionSupersededByURL = %v, want unchanged %q", got.DispositionSupersededByURL, supersededURL)
	}
	if got.DispositionRecordedAt == nil {
		t.Fatal("DispositionRecordedAt was cleared, want unchanged")
	}
}

// TestUpdateTaskPRDisposition_NeverTouchesSyncOwnedColumns pins the other
// half of the disjoint-writer guarantee: the disposition writer must never
// alter any sync-owned column. A disposition PATCH must not revert a
// freshly-synced state.
func TestUpdateTaskPRDisposition_NeverTouchesSyncOwnedColumns(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedDispositionTestTask(t, store, "task-disjoint-disposition", "ws-disjoint-disposition")

	now := time.Now().UTC()
	isDraft := false
	changedFiles := 42
	mergedBy := "carlosflorencio"
	closedBy := "nova28"
	tp := &TaskPR{
		TaskID:        "task-disjoint-disposition",
		Owner:         "kdlbs",
		Repo:          "kandev",
		PRNumber:      2,
		PRURL:         "https://github.com/kdlbs/kandev/pull/2",
		PRTitle:       "disjoint writer test 2",
		State:         "closed",
		ReviewState:   "approved",
		CreatedAt:     now,
		IsDraft:       &isDraft,
		ChangedFiles:  &changedFiles,
		MergedByLogin: &mergedBy,
		ClosedByLogin: &closedBy,
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}

	disposition := TaskPRDispositionDuplicate
	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, &disposition, nil, &now); err != nil {
		t.Fatalf("UpdateTaskPRDisposition: %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.Disposition == nil || *got.Disposition != TaskPRDispositionDuplicate {
		t.Fatalf("Disposition = %v, want %q", got.Disposition, TaskPRDispositionDuplicate)
	}
	if got.State != "closed" {
		t.Fatalf("State = %q, want unchanged %q (UpdateTaskPRDisposition must never touch sync-owned columns)", got.State, "closed")
	}
	if got.ReviewState != "approved" {
		t.Fatalf("ReviewState = %q, want unchanged %q", got.ReviewState, "approved")
	}
	if got.IsDraft == nil || *got.IsDraft != false {
		t.Fatalf("IsDraft = %v, want unchanged false", got.IsDraft)
	}
	if got.ChangedFiles == nil || *got.ChangedFiles != 42 {
		t.Fatalf("ChangedFiles = %v, want unchanged 42", got.ChangedFiles)
	}
	if got.MergedByLogin == nil || *got.MergedByLogin != "carlosflorencio" {
		t.Fatalf("MergedByLogin = %v, want unchanged", got.MergedByLogin)
	}
	if got.ClosedByLogin == nil || *got.ClosedByLogin != "nova28" {
		t.Fatalf("ClosedByLogin = %v, want unchanged", got.ClosedByLogin)
	}
}

// TestUpdateTaskPRDisposition_ClearsAllThreeColumnsInOneStatement covers
// AC-22: passing disposition == nil clears all three disposition columns to
// NULL in one statement, restoring the "nobody looked" state.
func TestUpdateTaskPRDisposition_ClearsAllThreeColumnsInOneStatement(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedDispositionTestTask(t, store, "task-clear-disposition", "ws-clear-disposition")

	now := time.Now().UTC()
	disposition := TaskPRDispositionWithdrawn
	tp := &TaskPR{
		TaskID:                "task-clear-disposition",
		Owner:                 "kdlbs",
		Repo:                  "kandev",
		PRNumber:              3,
		PRURL:                 "https://github.com/kdlbs/kandev/pull/3",
		PRTitle:               "clear disposition test",
		State:                 "closed",
		CreatedAt:             now,
		Disposition:           &disposition,
		DispositionRecordedAt: &now,
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}

	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, nil, nil, nil); err != nil {
		t.Fatalf("UpdateTaskPRDisposition clear: %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.Disposition != nil {
		t.Fatalf("Disposition = %v, want nil after clear", got.Disposition)
	}
	if got.DispositionSupersededByURL != nil {
		t.Fatalf("DispositionSupersededByURL = %v, want nil after clear", got.DispositionSupersededByURL)
	}
	if got.DispositionRecordedAt != nil {
		t.Fatalf("DispositionRecordedAt = %v, want nil after clear", got.DispositionRecordedAt)
	}
}

func seedDispositionTestTask(t *testing.T, store *Store, taskID, workspaceID string) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO workspaces (id) VALUES (?)`, workspaceID); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, taskID, workspaceID); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}
