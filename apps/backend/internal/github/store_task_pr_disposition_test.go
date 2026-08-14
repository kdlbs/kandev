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
	patch := DispositionPatch{DispositionSet: true, Disposition: &disposition, SupersededByURLSet: true}
	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, patch); err != nil {
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

	clearPatch := DispositionPatch{DispositionSet: true, SupersededByURLSet: true}
	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, clearPatch); err != nil {
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

// TestUpdateTaskPRDisposition_ConcurrentDisjointPatchesDoNotLoseWrites is the
// deterministic regression for SEC-001: the disposition writer used to merge
// a patch onto an app-level read of the row and then perform an absolute
// 3-column write of the merged result, so a patch touching only one column
// silently re-asserted the OTHER column's value from whatever the writer's
// own stale read happened to see. This drives store.UpdateTaskPRDisposition
// directly with two such stale reads — one patch setting only disposition,
// the other setting only superseded_by_url — landing in sequence, and
// asserts each patch's column survives the other's write intact. The fix
// makes every column write a CASE WHEN <field touched> THEN <new value> ELSE
// <column> END clause, so an untouched column is preserved by referencing
// its own live value inside the same statement rather than a merged
// snapshot (mirrors UpdateTaskCIOptions's boolPatchValue pattern).
func TestUpdateTaskPRDisposition_ConcurrentDisjointPatchesDoNotLoseWrites(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedDispositionTestTask(t, store, "task-disjoint-patches", "ws-disjoint-patches")

	initialDisposition := TaskPRDispositionDuplicate
	initialURL := "https://github.com/kdlbs/kandev/pull/900"
	now := time.Now().UTC()
	tp := &TaskPR{
		TaskID: "task-disjoint-patches", Owner: "kdlbs", Repo: "kandev", PRNumber: 4,
		PRURL: "https://github.com/kdlbs/kandev/pull/4", PRTitle: "disjoint patch race",
		State: "closed", CreatedAt: now,
		Disposition: &initialDisposition, DispositionSupersededByURL: &initialURL, DispositionRecordedAt: &now,
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}

	newDisposition := TaskPRDispositionSuperseded
	dispositionOnlyPatch := DispositionPatch{DispositionSet: true, Disposition: &newDisposition}
	newURL := "https://github.com/kdlbs/kandev/pull/901"
	urlOnlyPatch := DispositionPatch{SupersededByURLSet: true, SupersededByURL: &newURL}

	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, dispositionOnlyPatch); err != nil {
		t.Fatalf("UpdateTaskPRDisposition (disposition-only, lands first): %v", err)
	}
	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, urlOnlyPatch); err != nil {
		t.Fatalf("UpdateTaskPRDisposition (url-only, lands second): %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.Disposition == nil || *got.Disposition != TaskPRDispositionSuperseded {
		t.Fatalf("Disposition = %v, want %q preserved from the first write, not reverted by the second",
			got.Disposition, TaskPRDispositionSuperseded)
	}
	if got.DispositionSupersededByURL == nil || *got.DispositionSupersededByURL != newURL {
		t.Fatalf("DispositionSupersededByURL = %v, want %q from the second write", got.DispositionSupersededByURL, newURL)
	}
}

// TestUpdateTaskPRDisposition_URLOnlyPatchClearsURLWhenLiveDispositionIsNotSuperseded
// is the regression for the Testing-round finding: SEC-001's atomic per-column
// write fixed lost writes, but validateTaskPRDisposition only ever checks a
// per-request snapshot, not the row's state at write time. Two concurrent
// partial PATCHes that are each individually valid against the same stale
// snapshot can combine into a state AC-24 forbids: superseded_by_url non-nil
// while disposition is not "superseded". This seeds a row at
// disposition="superseded", lands a disposition-only write that moves it away
// from "superseded" (mirroring a request that read the row before this write
// landed and therefore validated fine), then lands a URL-only write (mirroring
// a second concurrent request that read the pre-first-write snapshot, so it
// also validated fine against a live "superseded" disposition that no longer
// holds by the time it writes). The fix must make the URL column's write
// depend on the row's LIVE disposition value inside the same statement — not
// the patch's own fields — so the second write clears the URL instead of
// setting it once the row is no longer superseded.
func TestUpdateTaskPRDisposition_URLOnlyPatchClearsURLWhenLiveDispositionIsNotSuperseded(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedDispositionTestTask(t, store, "task-invariant-race", "ws-invariant-race")

	initialDisposition := TaskPRDispositionSuperseded
	initialURL := "https://github.com/kdlbs/kandev/pull/900"
	now := time.Now().UTC()
	tp := &TaskPR{
		TaskID: "task-invariant-race", Owner: "kdlbs", Repo: "kandev", PRNumber: 5,
		PRURL: "https://github.com/kdlbs/kandev/pull/5", PRTitle: "invariant race",
		State: "closed", CreatedAt: now,
		Disposition: &initialDisposition, DispositionSupersededByURL: &initialURL, DispositionRecordedAt: &now,
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}

	// R1 (a well-behaved client changing disposition away from "superseded",
	// explicitly clearing the URL in the same patch — the only way to pass
	// AC-24 validation for this change): lands first.
	newDisposition := TaskPRDispositionDuplicate
	r1Patch := DispositionPatch{DispositionSet: true, Disposition: &newDisposition, SupersededByURLSet: true, SupersededByURL: nil}
	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, r1Patch); err != nil {
		t.Fatalf("UpdateTaskPRDisposition (R1, disposition away from superseded + clear URL): %v", err)
	}

	// R2 (a legitimate URL-only patch per AC-20, whose request-time validation
	// ran against the pre-R1 snapshot where disposition was still
	// "superseded"): lands second, after the live disposition has already
	// changed.
	newURL := "https://github.com/kdlbs/kandev/pull/901"
	r2Patch := DispositionPatch{SupersededByURLSet: true, SupersededByURL: &newURL}
	if err := store.UpdateTaskPRDisposition(ctx, tp.ID, r2Patch); err != nil {
		t.Fatalf("UpdateTaskPRDisposition (R2, url-only, lands second): %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.Disposition == nil || *got.Disposition != TaskPRDispositionDuplicate {
		t.Fatalf("Disposition = %v, want %q from R1", got.Disposition, TaskPRDispositionDuplicate)
	}
	if got.DispositionSupersededByURL != nil {
		t.Fatalf(
			"DispositionSupersededByURL = %v, want nil: disposition=%v is not %q, so a URL must never be "+
				"live on this row (AC-24) regardless of what R2's patch asked for",
			*got.DispositionSupersededByURL, *got.Disposition, TaskPRDispositionSuperseded,
		)
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
