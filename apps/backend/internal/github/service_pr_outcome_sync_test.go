package github

import (
	"context"
	"testing"
	"time"
)

func createOutcomeSyncTestTaskPR(t *testing.T, store *Store, tp *TaskPR) {
	t.Helper()
	if err := store.CreateTaskPR(context.Background(), tp); err != nil {
		t.Fatalf("create task PR: %v", err)
	}
}

// TestSyncTaskPR_PopulatedSyncWritesOutcomeFields covers AC-12: a populating
// sync writes is_draft, changed_files, and merged_by_login from the observed
// values.
func TestSyncTaskPR_PopulatedSyncWritesOutcomeFields(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Initial",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
	})

	status := &PRStatus{
		PR: &PR{
			Number: 1, State: "merged", RepoOwner: "owner", RepoName: "repo",
			Draft: false, IsDraftObserved: true,
			ChangedFiles: 8, ChangedFilesObserved: true,
			MergedByLogin: "carlosflorencio",
		},
		OutcomeFieldsPopulated: true,
	}
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("SyncTaskPR: %v", err)
	}

	got, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR: %v", err)
	}
	if got.IsDraft == nil || *got.IsDraft != false {
		t.Errorf("IsDraft = %v, want false", got.IsDraft)
	}
	if got.ChangedFiles == nil || *got.ChangedFiles != 8 {
		t.Errorf("ChangedFiles = %v, want 8", got.ChangedFiles)
	}
	if got.MergedByLogin == nil || *got.MergedByLogin != "carlosflorencio" {
		t.Errorf("MergedByLogin = %v, want carlosflorencio", got.MergedByLogin)
	}
}

// TestSyncTaskPR_NoMergerWritesNilNotEmptyString covers the nil-vs-empty
// rule: an upstream merged_by=null (empty MergedByLogin on PR) must persist
// as NULL, never "".
func TestSyncTaskPR_NoMergerWritesNilNotEmptyString(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Initial",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
	})

	status := &PRStatus{
		PR: &PR{
			Number: 1, State: "open", RepoOwner: "owner", RepoName: "repo",
			Draft: true, IsDraftObserved: true,
			ChangedFiles: 0, ChangedFilesObserved: true,
			MergedByLogin: "",
		},
		OutcomeFieldsPopulated: true,
	}
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("SyncTaskPR: %v", err)
	}

	got, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR: %v", err)
	}
	if got.MergedByLogin != nil {
		t.Errorf("MergedByLogin = %v, want nil (NULL), not empty string", *got.MergedByLogin)
	}
	if got.ChangedFiles == nil || *got.ChangedFiles != 0 {
		t.Errorf("ChangedFiles = %v, want 0 (real observation, not NULL)", got.ChangedFiles)
	}
}

// TestSyncTaskPR_UnpopulatedSyncPreservesStoredOutcomeValues covers AC-13: a
// non-populating sync leaves is_draft/changed_files/merged_by_login at their
// stored values, including a stored NULL and a previously-written non-NULL.
func TestSyncTaskPR_UnpopulatedSyncPreservesStoredOutcomeValues(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	isDraft := true
	changedFiles := 3
	mergedBy := "alice"
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Initial",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
		IsDraft: &isDraft, ChangedFiles: &changedFiles, MergedByLogin: &mergedBy,
	})

	// Unpopulated sync (e.g. the batched GraphQL branch-search path or list
	// results) must not touch the stored values, even though status.PR
	// carries different (zero) values.
	status := &PRStatus{
		PR: &PR{
			Number: 1, State: "open", RepoOwner: "owner", RepoName: "repo",
			Draft: false, ChangedFiles: 0, MergedByLogin: "",
		},
		OutcomeFieldsPopulated: false,
	}
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("SyncTaskPR: %v", err)
	}

	got, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR: %v", err)
	}
	if got.IsDraft == nil || *got.IsDraft != true {
		t.Errorf("IsDraft = %v, want preserved true", got.IsDraft)
	}
	if got.ChangedFiles == nil || *got.ChangedFiles != 3 {
		t.Errorf("ChangedFiles = %v, want preserved 3", got.ChangedFiles)
	}
	if got.MergedByLogin == nil || *got.MergedByLogin != "alice" {
		t.Errorf("MergedByLogin = %v, want preserved alice", got.MergedByLogin)
	}
}

// TestSyncTaskPR_ClosedByLoginFollowsClosureAttributionPopulated covers
// AC-14/AC-15: closed_by_login is written only when closure attribution was
// populated (the GraphQL path), and preserved otherwise.
func TestSyncTaskPR_ClosedByLoginFollowsClosureAttributionPopulated(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Initial",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
	})

	populated := &PRStatus{
		PR:                          &PR{Number: 1, State: "closed", RepoOwner: "owner", RepoName: "repo"},
		ClosureAttributionPopulated: true,
		ClosedByLogin:               "nova28",
	}
	if err := svc.SyncTaskPR(ctx, "t1", populated); err != nil {
		t.Fatalf("SyncTaskPR (populated): %v", err)
	}
	got, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR: %v", err)
	}
	if got.ClosedByLogin == nil || *got.ClosedByLogin != "nova28" {
		t.Fatalf("ClosedByLogin = %v, want nova28", got.ClosedByLogin)
	}

	// A later REST/gh CLI sync (unpopulated attribution) must not clear it.
	unpopulated := &PRStatus{
		PR:                          &PR{Number: 1, State: "closed", RepoOwner: "owner", RepoName: "repo"},
		ClosureAttributionPopulated: false,
	}
	if err := svc.SyncTaskPR(ctx, "t1", unpopulated); err != nil {
		t.Fatalf("SyncTaskPR (unpopulated): %v", err)
	}
	got2, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR after unpopulated sync: %v", err)
	}
	if got2.ClosedByLogin == nil || *got2.ClosedByLogin != "nova28" {
		t.Fatalf("ClosedByLogin after unpopulated sync = %v, want preserved nova28", got2.ClosedByLogin)
	}
}

// TestSyncTaskPR_AutoMergeObservedAtLatchesOnceAndNeverClears covers
// AC-16/AC-17: the latch sets once on the first armed observation, is not
// overwritten by a second armed observation, and survives a later sync that
// observes auto-merge disarmed or absent.
func TestSyncTaskPR_AutoMergeObservedAtLatchesOnceAndNeverClears(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Initial",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
	})

	armed := &PRStatus{
		PR:                     &PR{Number: 1, State: "open", RepoOwner: "owner", RepoName: "repo", AutoMergeEnabled: true},
		OutcomeFieldsPopulated: true,
	}
	if err := svc.SyncTaskPR(ctx, "t1", armed); err != nil {
		t.Fatalf("SyncTaskPR (armed): %v", err)
	}
	first, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR: %v", err)
	}
	if first.AutoMergeObservedAt == nil {
		t.Fatal("AutoMergeObservedAt = nil, want set after first armed observation")
	}
	firstObservedAt := *first.AutoMergeObservedAt

	// A later sync observing auto-merge disarmed must not clear the latch.
	disarmed := &PRStatus{
		PR:                     &PR{Number: 1, State: "open", RepoOwner: "owner", RepoName: "repo", AutoMergeEnabled: false},
		OutcomeFieldsPopulated: true,
	}
	if err := svc.SyncTaskPR(ctx, "t1", disarmed); err != nil {
		t.Fatalf("SyncTaskPR (disarmed): %v", err)
	}
	second, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR after disarmed sync: %v", err)
	}
	if second.AutoMergeObservedAt == nil || !second.AutoMergeObservedAt.Equal(firstObservedAt) {
		t.Fatalf("AutoMergeObservedAt after disarmed sync = %v, want unchanged %v", second.AutoMergeObservedAt, firstObservedAt)
	}

	// A later sync observing auto-merge armed again must not overwrite the
	// latch with a new timestamp.
	time.Sleep(2 * time.Millisecond)
	armedAgain := &PRStatus{
		PR:                     &PR{Number: 1, State: "open", RepoOwner: "owner", RepoName: "repo", AutoMergeEnabled: true},
		OutcomeFieldsPopulated: true,
	}
	if err := svc.SyncTaskPR(ctx, "t1", armedAgain); err != nil {
		t.Fatalf("SyncTaskPR (armed again): %v", err)
	}
	third, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR after second armed sync: %v", err)
	}
	if third.AutoMergeObservedAt == nil || !third.AutoMergeObservedAt.Equal(firstObservedAt) {
		t.Fatalf("AutoMergeObservedAt after second armed sync = %v, want still %v", third.AutoMergeObservedAt, firstObservedAt)
	}
}

// TestUpdateTaskPR_AutoMergeLatchIsAtomicUnderConcurrentStaleWrites is the
// deterministic regression for the race SyncTaskPR's own read-then-write
// shape can't rule out: two syncs can both read AutoMergeObservedAt as NULL
// (neither has observed the other's write yet) and independently compute
// their own "now" timestamp before either writes. This drives
// store.UpdateTaskPR directly with two such stale in-memory TaskPR copies —
// SyncTaskPR always re-reads fresh at the top of each call, so it cannot
// itself reproduce two readers racing off the same NULL snapshot — and
// asserts the SQL-level COALESCE(auto_merge_observed_at, ?) means whichever
// write actually lands first wins, with the second writer's differing
// timestamp silently discarded rather than overwriting it.
func TestUpdateTaskPR_AutoMergeLatchIsAtomicUnderConcurrentStaleWrites(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	base := &TaskPR{
		TaskID: "task-latch-race", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Initial",
		HeadBranch: "feat", BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, base); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Two independent in-memory copies of the same row, both taken while
	// AutoMergeObservedAt was still NULL — simulating two syncs that raced
	// off the same stale snapshot and each computed their own timestamp.
	writerA := *base
	firstObservedAt := time.Now().UTC()
	writerA.AutoMergeObservedAt = &firstObservedAt

	writerB := *base
	secondObservedAt := firstObservedAt.Add(time.Minute)
	writerB.AutoMergeObservedAt = &secondObservedAt

	if err := store.UpdateTaskPR(ctx, &writerA); err != nil {
		t.Fatalf("UpdateTaskPR (writer A, lands first): %v", err)
	}
	if err := store.UpdateTaskPR(ctx, &writerB); err != nil {
		t.Fatalf("UpdateTaskPR (writer B, lands second, stale nil-based timestamp): %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, base.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.AutoMergeObservedAt == nil || !got.AutoMergeObservedAt.Equal(firstObservedAt) {
		t.Fatalf("AutoMergeObservedAt = %v, want the first writer's timestamp %v preserved, not the second writer's %v",
			got.AutoMergeObservedAt, firstObservedAt, secondObservedAt)
	}
}

// TestPersistAndPublishTaskPRSync_PublishesReReadValueNotStaleInMemoryOne is
// the deterministic regression for codex [P2]. SyncTaskPR computes
// tp.AutoMergeObservedAt from its OWN top-of-call read, but UpdateTaskPR
// persists that field through COALESCE(auto_merge_observed_at, ?), so a
// concurrent sync landing its own write in the gap between this call's read
// and its write can leave the column holding a different, earlier timestamp
// than tp carries in memory. Reproducing the exact interleaving would need
// real goroutines racing a synchronous function with no injectable pause
// point, which is nondeterministic; this instead drives the extracted
// persistAndPublishTaskPRSync helper directly with a manufactured "stale"
// tp (as if this call's read happened before a concurrent write it never
// observed) after seeding the DB with a different, already-persisted value
// (the concurrent write). The bug: publishing tp unmodified would broadcast
// the stale value, not what a fresh read returns.
func TestPersistAndPublishTaskPRSync_PublishesReReadValueNotStaleInMemoryOne(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	tp := &TaskPR{
		TaskID: "task-p2-race", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "p2 race",
		HeadBranch: "feat", BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Simulate a concurrent sync that already landed its own write: the DB
	// now holds persistedAt, set while our own in-memory tp (below) still
	// reflects the pre-race NULL it read moments earlier.
	concurrentWriter := *tp
	persistedAt := time.Now().UTC()
	concurrentWriter.AutoMergeObservedAt = &persistedAt
	if err := store.UpdateTaskPR(ctx, &concurrentWriter); err != nil {
		t.Fatalf("simulate concurrent writer: %v", err)
	}

	// Our own call's stale view: computed from a read taken before the
	// concurrent writer's timestamp above landed.
	staleTP := *tp
	staleObservedAt := persistedAt.Add(-time.Minute)
	staleTP.AutoMergeObservedAt = &staleObservedAt

	if err := svc.persistAndPublishTaskPRSync(ctx, &staleTP, nil, true, true); err != nil {
		t.Fatalf("persistAndPublishTaskPRSync: %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.AutoMergeObservedAt == nil || !got.AutoMergeObservedAt.Equal(persistedAt) {
		t.Fatalf("persisted AutoMergeObservedAt = %v, want the concurrent writer's %v preserved by COALESCE",
			got.AutoMergeObservedAt, persistedAt)
	}

	if eb.publishedCount() != 1 {
		t.Fatalf("published events = %d, want 1", eb.publishedCount())
	}
	published, ok := eb.events[0].Data.(*TaskPR)
	if !ok {
		t.Fatalf("published event data = %T, want *TaskPR", eb.events[0].Data)
	}
	if published.AutoMergeObservedAt == nil || !published.AutoMergeObservedAt.Equal(persistedAt) {
		t.Fatalf("published AutoMergeObservedAt = %v, want the re-read persisted value %v, not the stale in-memory %v",
			published.AutoMergeObservedAt, persistedAt, staleObservedAt)
	}
}

// TestSyncTaskPR_OutcomeFieldChangePublishesEvent covers AC-18: a sync that
// changes only an outcome field (no legacy field change) still publishes
// github.task_pr.updated, and an unchanged sync publishes nothing.
//
// The seed and status deliberately match on every non-outcome comparison
// SyncTaskPR's `changed` expression checks, including mergeable_state: a
// draft PR forces prepareTaskPRSyncState to override mergeableState to
// "draft" regardless of status.MergeableState, so an earlier version of this
// test (Draft: true, tp.MergeableState left at its zero value "") had TWO
// independent causes of changed == true — the mergeableState override and
// the outcome-field change — and would have stayed green even if
// taskPROutcomeFieldsChanged were deleted from that expression entirely.
// Using a non-draft PR with a pre-matched, non-empty MergeableState removes
// that second cause, so changedFiles is the only thing that can make this
// sync report changed.
func TestSyncTaskPR_OutcomeFieldChangePublishesEvent(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()
	seededIsDraft := false
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Same title",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
		MergeableState: "clean", IsDraft: &seededIsDraft,
	})

	status := &PRStatus{
		PR: &PR{
			Number: 1, Title: "Same title", State: "open", RepoOwner: "owner", RepoName: "repo",
			Draft: false, IsDraftObserved: true,
			ChangedFiles: 5, ChangedFilesObserved: true,
			MergedByLogin: "",
		},
		MergeableState:         "clean",
		OutcomeFieldsPopulated: true,
	}
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("SyncTaskPR (change): %v", err)
	}
	if eb.publishedCount() != 1 {
		t.Fatalf("published events = %d, want 1 after an outcome-only change", eb.publishedCount())
	}

	// Re-syncing identical values must not publish again.
	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("SyncTaskPR (no change): %v", err)
	}
	if eb.publishedCount() != 1 {
		t.Fatalf("published events = %d, want still 1 after an unchanged sync", eb.publishedCount())
	}
}
