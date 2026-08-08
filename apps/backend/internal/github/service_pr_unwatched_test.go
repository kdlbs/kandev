package github

import (
	"context"
	"testing"
	"time"
)

// seedUnwatchedTaskPR inserts a linked TaskPR row that no watch points at,
// last synced long enough ago to be outside PRSyncFreshnessWindow.
func seedUnwatchedTaskPR(t *testing.T, store *Store, taskID string, number int, branch string) {
	t.Helper()
	now := time.Now().UTC()
	staleSync := now.Add(-1 * time.Hour)
	if err := store.CreateTaskPR(context.Background(), &TaskPR{
		TaskID:       taskID,
		WorkspaceID:  testWorkspaceID,
		RepositoryID: "repo-1",
		Owner:        "owner",
		Repo:         "repo",
		PRNumber:     number,
		PRURL:        "https://github.com/owner/repo/pull/1293",
		PRTitle:      "Left behind",
		HeadBranch:   branch,
		BaseBranch:   "main",
		State:        "open",
		ChecksState:  "success",
		CreatedAt:    now.Add(-3 * time.Hour),
		LastSyncedAt: &staleSync,
	}); err != nil {
		t.Fatalf("seed unwatched task PR: %v", err)
	}
}

// TestTriggerPRSyncAll_RefreshesPRLeftBehindByBranchHandover is the
// regression for the stale multi-PR status bug: a task's PR watch is unique
// per (session, repository, branch), so when the session moves to a second
// branch the watch is re-pointed and the first branch's PR loses its only
// sync handle. It then keeps `state=open` forever — the UI renders an
// already-merged PR as green, mergeable, with a live merge button.
func TestTriggerPRSyncAll_RefreshesPRLeftBehindByBranchHandover(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	// PR #1293 was linked from the first branch and is stuck at "open".
	seedUnwatchedTaskPR(t, store, "task-1", 1293, "feature/first")

	// The task's only watch has since moved to the second branch's PR.
	if _, err := svc.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 1299, "feature/second",
	); err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}

	now := time.Now().UTC()
	mergedAt := now.Add(-10 * time.Minute)
	mockClient.AddPR(&PR{
		Number: 1293, Title: "Left behind", State: "merged",
		HTMLURL:    "https://github.com/owner/repo/pull/1293",
		HeadBranch: "feature/first", BaseBranch: "main",
		RepoOwner: "owner", RepoName: "repo",
		CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: mergedAt, MergedAt: &mergedAt,
	})
	mockClient.AddPR(&PR{
		Number: 1299, Title: "Current", State: "open",
		HTMLURL:    "https://github.com/owner/repo/pull/1299",
		HeadBranch: "feature/second", BaseBranch: "main",
		RepoOwner: "owner", RepoName: "repo",
		CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now,
	})

	prs, err := svc.TriggerPRSyncAll(ctx, "task-1")
	if err != nil {
		t.Fatalf("TriggerPRSyncAll: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("expected both PR rows returned, got %d", len(prs))
	}

	left, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	if left == nil {
		t.Fatal("expected the left-behind PR row to still exist")
	}
	if left.State != prStateMerged {
		t.Errorf("left-behind PR #1293 state = %q, want %q", left.State, prStateMerged)
	}
	if left.MergedAt == nil {
		t.Error("left-behind PR #1293 must record merged_at once reconciled")
	}

	// The returned slice is what the WS handler hands the frontend, so the
	// merged state has to be visible there too — not only in the DB.
	for _, tp := range prs {
		if tp.PRNumber == 1293 && tp.State != prStateMerged {
			t.Errorf("returned PR #1293 state = %q, want %q", tp.State, prStateMerged)
		}
	}
}

// TestTriggerPRSyncAll_RefreshesTaskPRWithNoWatchAtAll covers rows that never
// had a watch — a task created from a PR URL. Before the fix the no-watch
// branch returned the stored rows verbatim, so those PRs never synced once.
func TestTriggerPRSyncAll_RefreshesTaskPRWithNoWatchAtAll(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedUnwatchedTaskPR(t, store, "task-1", 1293, "feature/first")

	now := time.Now().UTC()
	closedAt := now.Add(-5 * time.Minute)
	mockClient.AddPR(&PR{
		Number: 1293, Title: "Left behind", State: "closed",
		HTMLURL:    "https://github.com/owner/repo/pull/1293",
		HeadBranch: "feature/first", BaseBranch: "main",
		RepoOwner: "owner", RepoName: "repo",
		CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: closedAt, ClosedAt: &closedAt,
	})

	if _, err := svc.TriggerPRSyncAll(ctx, "task-1"); err != nil {
		t.Fatalf("TriggerPRSyncAll: %v", err)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	if got == nil || got.State != prStateClosed {
		t.Fatalf("expected watch-less PR row to reach %q, got %+v", prStateClosed, got)
	}
}

// TestRefreshStaleWorkspaceWatches_HealsUnwatchedRow covers the surface the
// kanban board and the topbar aggregate read from: the workspace-wide
// background refresh (driven by ListWorkspaceTaskPRs) fans in watches only,
// so a PR left behind by a branch handover stayed stale on every workspace
// load. Also the only test that drives the batched GraphQL branch of the
// unwatched sync.
func TestRefreshStaleWorkspaceWatches_HealsUnwatchedRow(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedUnwatchedTaskPR(t, store, "task-1", 1293, "feature/first")

	// The unwatched group is reconciled before the watch batch, so the canned
	// responses are consumed in that order.
	gh.prResponses = []string{
		batchedMergedPRResponse("feature/first", "2026-01-02T00:00:00Z"),
		batchedMergedPRResponse("feature/second", ""),
	}
	if _, err := svc.CreatePRWatchForWorkspace(
		ctx, testWorkspaceID, "session-1", "task-1", "repo-1", "owner", "repo", 1299, "feature/second",
	); err != nil {
		t.Fatalf("CreatePRWatch: %v", err)
	}

	// ListWorkspaceTaskPRs derives this stale set from last_synced_at and then
	// hands it to the background goroutine; the join it uses needs the real
	// tasks schema, so drive the goroutine directly.
	svc.refreshStaleWorkspaceWatches(testWorkspaceID, map[string]struct{}{"task-1": {}})
	waitForWorkspaceRefresh(t, svc, testWorkspaceID)

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	if got == nil || got.State != prStateMerged {
		t.Fatalf("expected background refresh to reach %q for the unwatched PR, got %+v", prStateMerged, got)
	}
}

// batchedMergedPRResponse builds a single-alias batched PR query response.
// A non-empty mergedAt makes convertBatchedPRResult classify it as merged.
func batchedMergedPRResponse(headBranch, mergedAt string) string {
	return `{"data":{"repo0":{"pr0":{
		"state": "MERGED", "title": "PR", "url": "https://x/1",
		"isDraft": false, "mergeable": "UNKNOWN", "mergeStateStatus": "UNKNOWN",
		"headRefName": "` + headBranch + `", "baseRefName": "main", "headRefOid": "abc",
		"author": {"login": "alice"},
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z",
		"mergedAt": "` + mergedAt + `",
		"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
		"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}
	}}}}`
}

// waitForWorkspaceRefresh blocks until the background refresh goroutine for
// the workspace has cleared its singleflight key.
func waitForWorkspaceRefresh(t *testing.T, svc *Service, workspaceID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, busy := svc.inflightWorkspaceRefreshes.Load(workspaceID); !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("background refresh for %s never completed", workspaceID)
}

// TestTriggerPRSyncAll_UnwatchedReconcilePreservesAggregates locks the scope
// of the unwatched path: it converges lifecycle state only. Check and review
// aggregates belong to the watch-driven sync, which fetches the reviews and
// check runs needed to compute them — reconciling them from here would persist
// a less-informed answer, because the per-PR REST status marks its counts
// populated even when it has nothing to count. E2E caught this as six PR
// popover specs losing their seeded 27/22 check counts to 0/0.
func TestTriggerPRSyncAll_UnwatchedReconcilePreservesAggregates(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)

	now := time.Now().UTC()
	staleSync := now.Add(-1 * time.Hour)
	required := 2
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", WorkspaceID: testWorkspaceID, RepositoryID: "repo-1",
		Owner: "owner", Repo: "repo", PRNumber: 1293,
		PRURL: "https://github.com/owner/repo/pull/1293", PRTitle: "Left behind",
		HeadBranch: "feature/first", BaseBranch: "main", State: "open",
		// Aggregates a richer sync already computed.
		ChecksState: "failure", ChecksTotal: 27, ChecksPassing: 22,
		ReviewState: "approved", ReviewCount: 1, PendingReviewCount: 0,
		RequiredReviews: &required, UnresolvedReviewThreads: 3,
		MergeableState: "blocked",
		CreatedAt:      now.Add(-3 * time.Hour), LastSyncedAt: &staleSync,
	}); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	// The provider knows the PR merged but carries no reviews and no check
	// runs — exactly the shape that zeroed the aggregates before this fix.
	mergedAt := now.Add(-10 * time.Minute)
	mockClient.AddPR(&PR{
		Number: 1293, Title: "Left behind", State: prStateMerged,
		HTMLURL:    "https://github.com/owner/repo/pull/1293",
		HeadBranch: "feature/first", BaseBranch: "main",
		RepoOwner: "owner", RepoName: "repo",
		CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: mergedAt, MergedAt: &mergedAt,
	})

	if _, err := svc.TriggerPRSyncAll(ctx, "task-1"); err != nil {
		t.Fatalf("TriggerPRSyncAll: %v", err)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: err=%v row=%v", err, got)
	}
	// Lifecycle converged...
	if got.State != prStateMerged || got.MergedAt == nil {
		t.Errorf("state=%q merged_at=%v, want merged with a timestamp", got.State, got.MergedAt)
	}
	// ...and nothing else was touched.
	if got.ChecksTotal != 27 || got.ChecksPassing != 22 {
		t.Errorf("checks %d/%d, want 27/22 preserved", got.ChecksPassing, got.ChecksTotal)
	}
	if got.ChecksState != "failure" {
		t.Errorf("checks_state = %q, want %q preserved", got.ChecksState, "failure")
	}
	if got.ReviewState != "approved" || got.ReviewCount != 1 {
		t.Errorf("review_state=%q count=%d, want approved/1 preserved", got.ReviewState, got.ReviewCount)
	}
	if got.UnresolvedReviewThreads != 3 {
		t.Errorf("unresolved_review_threads = %d, want 3 preserved", got.UnresolvedReviewThreads)
	}
	if got.RequiredReviews == nil || *got.RequiredReviews != 2 {
		t.Errorf("required_reviews = %v, want 2 preserved", got.RequiredReviews)
	}
	if got.MergeableState != "blocked" {
		t.Errorf("mergeable_state = %q, want %q preserved", got.MergeableState, "blocked")
	}
}

func TestUnwatchedTaskPRs_Selection(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-1 * time.Hour)
	fresh := now.Add(-1 * time.Second)
	detached := now.Add(-2 * time.Hour)

	row := func(number int, state string, syncedAt *time.Time) *TaskPR {
		return &TaskPR{
			TaskID: "task-1", Owner: "owner", Repo: "repo",
			PRNumber: number, State: state, LastSyncedAt: syncedAt,
		}
	}
	detachedRow := row(7, "open", &stale)
	detachedRow.DetachedAt = &detached

	rows := []*TaskPR{
		row(1, "open", &stale),        // stale + unwatched -> selected
		row(2, "open", &stale),        // watched -> skipped
		row(3, prStateMerged, &stale), // terminal -> skipped
		row(4, prStateClosed, &stale), // terminal -> skipped
		row(5, "open", &fresh),        // inside freshness window -> skipped
		row(6, "open", nil),           // never synced -> selected
		detachedRow,                   // detached -> skipped
	}
	watches := []*PRWatch{
		{Owner: "owner", Repo: "repo", PRNumber: 2},
		{Owner: "owner", Repo: "repo", PRNumber: 0}, // searching watch covers nothing
		nil,
	}

	got := unwatchedTaskPRs(rows, watches, now)
	if len(got) != 2 {
		t.Fatalf("expected 2 selected rows, got %d (%+v)", len(got), got)
	}
	if got[0].PRNumber != 1 || got[1].PRNumber != 6 {
		t.Errorf("selected PRs = %d, %d; want 1, 6", got[0].PRNumber, got[1].PRNumber)
	}
}
