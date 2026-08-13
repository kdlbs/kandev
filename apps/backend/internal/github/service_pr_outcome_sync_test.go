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
			Draft: false, ChangedFiles: 8, MergedByLogin: "carlosflorencio",
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
			Draft: true, ChangedFiles: 0, MergedByLogin: "",
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

// TestSyncTaskPR_OutcomeFieldChangePublishesEvent covers AC-18: a sync that
// changes only an outcome field (no legacy field change) still publishes
// github.task_pr.updated, and an unchanged sync publishes nothing.
func TestSyncTaskPR_OutcomeFieldChangePublishesEvent(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Same title",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
	})

	status := &PRStatus{
		PR: &PR{
			Number: 1, Title: "Same title", State: "open", RepoOwner: "owner", RepoName: "repo",
			Draft: true, ChangedFiles: 5, MergedByLogin: "",
		},
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
