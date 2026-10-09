package github

import (
	"context"
	"testing"
	"time"
)

func TestPRStatusFromTaskPRSnapshotPreservesDraftConflictObservation(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, svc.store, "task-snapshot-conflict", false)

	conflict := true
	draft := true
	snapshot := &TaskPR{
		TaskID: "task-snapshot-conflict", WorkspaceID: testWorkspaceID, RepositoryID: "repo-1",
		Owner: "owner", Repo: "repo", PRNumber: 123, State: prStateOpen,
		MergeableState: "draft", IsDraft: &draft, HasMergeConflicts: &conflict,
	}
	status := prStatusFromTaskPRSnapshot(snapshot)
	existing := &TaskPR{
		TaskID: snapshot.TaskID, WorkspaceID: testWorkspaceID, RepositoryID: "repo-1",
		Owner: "owner", Repo: "repo", PRNumber: snapshot.PRNumber,
		State: prStateOpen, MergeableState: "draft", HasMergeConflicts: &conflict,
	}

	next := svc.prepareTaskPRSyncState(ctx, existing, status)
	if next.hasMergeConflicts == nil || !*next.hasMergeConflicts {
		t.Fatalf("draft snapshot conflict = %v, want true", next.hasMergeConflicts)
	}

	seedTask(t, svc.store, "task-snapshot-conflict-new", false)
	associated, err := svc.associatePRWithTask(
		ctx, testWorkspaceID, "task-snapshot-conflict-new", "repo-1", status.PR,
		false, false, TaskPRSourceWatch,
	)
	if err != nil {
		t.Fatalf("associate snapshot PR: %v", err)
	}
	if associated.HasMergeConflicts == nil || !*associated.HasMergeConflicts {
		t.Fatalf("associated draft snapshot conflict = %v, want true", associated.HasMergeConflicts)
	}
	if associated.MergeableState != "draft" {
		t.Fatalf("associated mergeable state = %q, want draft", associated.MergeableState)
	}
}

func TestUnwatchedReconcileUpdatesConflictWithoutReplacingAggregates(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-unwatched-conflict", false)
	now := time.Now().UTC()
	staleSync := now.Add(-time.Hour)
	conflict := true
	requiredReviews := 2
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-unwatched-conflict", WorkspaceID: testWorkspaceID, RepositoryID: "repo-1",
		Owner: "owner", Repo: "repo", PRNumber: 124, State: prStateOpen,
		ChecksState: "failure", ChecksTotal: 5, ChecksPassing: 3,
		ReviewState: "approved", ReviewCount: 1, PendingReviewCount: 0,
		RequiredReviews: &requiredReviews, UnresolvedReviewThreads: 4,
		MergeableState: "blocked", HasMergeConflicts: &conflict,
		LastSyncedAt: &staleSync,
	}); err != nil {
		t.Fatalf("seed task PR: %v", err)
	}
	tp, err := store.GetTaskPRByRepoAndNumber(ctx, "task-unwatched-conflict", "repo-1", 124)
	if err != nil || tp == nil {
		t.Fatalf("load task PR: err=%v row=%v", err, tp)
	}

	updated := subscribeTaskPRUpdated(t, svc)
	status := &PRStatus{PR: &PR{
		Number: 124, State: prStateOpen, MergeableState: "clean",
		RepoOwner: "owner", RepoName: "repo",
	}}
	if err := svc.reconcileTaskPRLifecycle(ctx, tp, status); err != nil {
		t.Fatalf("clear conflict observation: %v", err)
	}
	cleared := awaitTaskPRUpdated(t, updated)
	if cleared.HasMergeConflicts == nil || *cleared.HasMergeConflicts {
		t.Fatalf("cleared conflict = %v, want false", cleared.HasMergeConflicts)
	}
	if cleared.ChecksState != "failure" || cleared.ChecksTotal != 5 || cleared.ChecksPassing != 3 {
		t.Fatalf("checks aggregate changed during conflict sync: state=%q %d/%d", cleared.ChecksState, cleared.ChecksPassing, cleared.ChecksTotal)
	}
	if cleared.ReviewState != "approved" || cleared.ReviewCount != 1 || cleared.UnresolvedReviewThreads != 4 {
		t.Fatalf("review aggregate changed during conflict sync: state=%q count=%d unresolved=%d", cleared.ReviewState, cleared.ReviewCount, cleared.UnresolvedReviewThreads)
	}

	status.PR.MergeableState = "dirty"
	if err := svc.reconcileTaskPRLifecycle(ctx, cleared, status); err != nil {
		t.Fatalf("observe conflict: %v", err)
	}
	conflicted := awaitTaskPRUpdated(t, updated)
	if conflicted.HasMergeConflicts == nil || !*conflicted.HasMergeConflicts {
		t.Fatalf("conflict observation = %v, want true", conflicted.HasMergeConflicts)
	}
}
