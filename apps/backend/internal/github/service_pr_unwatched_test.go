package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
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

	// Subscribe before triggering: the published event is the observable side
	// effect of the row being written, so it's a deterministic join.
	updated := subscribeTaskPRUpdated(t, svc)

	// ListWorkspaceTaskPRs derives this stale set from last_synced_at and then
	// hands it to the background goroutine; the join it uses needs the real
	// tasks schema, so drive the goroutine directly.
	svc.refreshStaleWorkspaceWatches(testWorkspaceID, map[string]struct{}{"task-1": {}})
	unwatchedUpdate := awaitTaskPRUpdated(t, updated)
	if unwatchedUpdate.PRNumber != 1293 {
		t.Fatalf("first background update PR = %d, want unwatched PR 1293", unwatchedUpdate.PRNumber)
	}
	// The same goroutine updates the watched PR after the unwatched row. Wait
	// for that write too: canceling it via Stop can make sqlite discard the
	// sole :memory: connection and leave the final assertion without a schema.
	watchedUpdate := awaitTaskPRUpdated(t, updated)
	if watchedUpdate.PRNumber != 1299 {
		t.Fatalf("second background update PR = %d, want watched PR 1299", watchedUpdate.PRNumber)
	}
	// Drain the goroutine before the in-memory DB closes. Both observable writes
	// have completed, so Stop has no database operation left to cancel.
	svc.Stop()

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	if got == nil || got.State != prStateMerged {
		t.Fatalf("expected background refresh to reach %q for the unwatched PR, got %+v", prStateMerged, got)
	}
}

// TestUnwatchedReconcile_FallsBackPerPRWhenBatchFails covers the branch a
// GraphQL-capable client takes when the batch query itself fails (auth blip,
// network error): graphQLExecutorFor succeeds, runBatchedPRQuery does not, and
// the per-PR GetPR path has to carry the reconcile.
func TestUnwatchedReconcile_FallsBackPerPRWhenBatchFails(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedUnwatchedTaskPR(t, store, "task-1", 1293, "feature/first")

	// GraphQL-capable client whose batch query fails outright.
	gh.prErr = errors.New("graphql unavailable")
	now := time.Now().UTC()
	mergedAt := now.Add(-10 * time.Minute)
	gh.AddPR(&PR{
		Number: 1293, Title: "Left behind", State: prStateMerged,
		HTMLURL:    "https://github.com/owner/repo/pull/1293",
		HeadBranch: "feature/first", BaseBranch: "main",
		RepoOwner: "owner", RepoName: "repo",
		CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: mergedAt, MergedAt: &mergedAt,
	})

	if _, err := svc.TriggerPRSyncAll(ctx, "task-1"); err != nil {
		t.Fatalf("TriggerPRSyncAll: %v", err)
	}
	if len(gh.prQueries) == 0 {
		t.Fatal("expected the batched query to be attempted before the fallback")
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: err=%v row=%v", err, got)
	}
	if got.State != prStateMerged || got.MergedAt == nil {
		t.Fatalf("per-PR fallback did not reconcile: state=%q merged_at=%v", got.State, got.MergedAt)
	}
}

// TestUnwatchedReconcile_CoalescesConcurrentFetches locks the singleflight on
// the unwatched batched fetch. The on-demand sync, the workspace background
// refresh, and a second browser tab can all want the same ref set at once;
// without coalescing each issues its own GraphQL call against the same gh
// throttle.
func TestUnwatchedReconcile_CoalescesConcurrentFetches(t *testing.T) {
	_, svc, gh, store := setupBatchedPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedUnwatchedTaskPR(t, store, "task-1", 1293, "feature/first")

	// Two canned responses so a second (unwanted) call fails the count
	// assertion rather than starving on missing data.
	gh.prResponses = []string{
		batchedMergedPRResponse("feature/first", "2026-01-02T00:00:00Z"),
		batchedMergedPRResponse("feature/first", "2026-01-02T00:00:00Z"),
	}
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	gh.onExecute = func() {
		started <- struct{}{}
		<-release
	}

	done := make(chan struct{}, 2)
	go func() { _, _ = svc.TriggerPRSyncAll(ctx, "task-1"); done <- struct{}{} }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first reconcile never entered ExecuteGraphQL")
	}

	// Second caller must join the in-flight fetch rather than start its own.
	go func() { _, _ = svc.TriggerPRSyncAll(ctx, "task-1"); done <- struct{}{} }()
	select {
	case <-started:
		t.Fatal("second reconcile issued its own GraphQL call — singleflight broken")
	case <-time.After(100 * time.Millisecond):
		// Bounded negative assertion, per apps/backend/AGENTS.md.
	}

	close(release)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("reconcile goroutines did not finish")
		}
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil || got == nil || got.State != prStateMerged {
		t.Fatalf("expected the shared flight to reconcile the row: err=%v row=%+v", err, got)
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

// subscribeTaskPRUpdated returns a channel fed by github.task_pr.updated
// events. Channel synchronization rather than a polling sleep: the background
// refresh does no subprocess work, so its write is observable through the
// event it publishes (see apps/backend/AGENTS.md, "Joining production
// goroutines in tests").
func subscribeTaskPRUpdated(t *testing.T, svc *Service) <-chan *TaskPR {
	t.Helper()
	seen := make(chan *TaskPR, 4)
	sub, err := svc.eventBus.Subscribe(events.GitHubTaskPRUpdated, func(_ context.Context, e *bus.Event) error {
		tp, _ := e.Data.(*TaskPR)
		select {
		case seen <- tp:
		default: // buffered channel full: the join only needs the first event
		}
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe to %s: %v", events.GitHubTaskPRUpdated, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return seen
}

// awaitTaskPRUpdated blocks until the reconcile publishes, failing fast rather
// than letting a missing event stall the package's test timeout.
func awaitTaskPRUpdated(t *testing.T, updated <-chan *TaskPR) *TaskPR {
	t.Helper()
	select {
	case tp := <-updated:
		return tp
	case <-time.After(5 * time.Second):
		t.Fatal("no github.task_pr.updated event published — reconcile never wrote the row")
		return nil
	}
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

// TestTriggerPRSyncAll_UnwatchedReconcileWritesOutcomeAttribution is the
// regression for AC-36: a PR that merges with no active watch is reconciled
// only through syncUnwatchedTaskPRGroup, never through SyncTaskPR. Before
// this fix, reconcileTaskPRLifecycle discarded the already-fetched draft
// status and merger identity, so the row went terminal with
// merged_by_login/is_draft still NULL — merged_at >= activation with a NULL
// merged_by_login is exactly the writer-fault state AC-36 forbids.
func TestTriggerPRSyncAll_UnwatchedReconcileWritesOutcomeAttribution(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedUnwatchedTaskPR(t, store, "task-1", 1293, "feature/first")

	mergedAt := time.Now().UTC().Add(-10 * time.Minute)
	mockClient.AddPR(&PR{
		Number: 1293, Title: "Left behind", State: prStateMerged,
		HTMLURL:    "https://github.com/owner/repo/pull/1293",
		HeadBranch: "feature/first", BaseBranch: "main",
		RepoOwner: "owner", RepoName: "repo",
		CreatedAt: mergedAt.Add(-2 * time.Hour), UpdatedAt: mergedAt, MergedAt: &mergedAt,
		Draft: false, ChangedFiles: 4, MergedByLogin: "carlosflorencio",
	})

	if _, err := svc.TriggerPRSyncAll(ctx, "task-1"); err != nil {
		t.Fatalf("TriggerPRSyncAll: %v", err)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 1293)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: err=%v row=%v", err, got)
	}
	if got.State != prStateMerged || got.MergedAt == nil {
		t.Fatalf("state=%q merged_at=%v, want merged with a timestamp", got.State, got.MergedAt)
	}
	if got.MergedByLogin == nil || *got.MergedByLogin != "carlosflorencio" {
		t.Errorf("merged_by_login = %v, want %q (AC-36 requires non-NULL once merged_at is set)",
			got.MergedByLogin, "carlosflorencio")
	}
	if got.IsDraft == nil || *got.IsDraft != false {
		t.Errorf("is_draft = %v, want false observed, not NULL", got.IsDraft)
	}
	if got.ChangedFiles == nil || *got.ChangedFiles != 4 {
		t.Errorf("changed_files = %v, want 4 observed, not NULL", got.ChangedFiles)
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
