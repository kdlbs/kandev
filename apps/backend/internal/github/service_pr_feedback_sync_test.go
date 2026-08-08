package github

import (
	"context"
	"testing"
	"time"
)

const (
	feedbackSyncOwner = "owner"
	feedbackSyncRepo  = "repo"
	feedbackSyncHead  = "sha-feedback"
)

// seedFeedbackTaskPR inserts a linked TaskPR row for the feedback-sync tests,
// applying `mutate` so each test can dirty the fields it cares about.
func seedFeedbackTaskPR(t *testing.T, store *Store, taskID string, number int, mutate func(*TaskPR)) {
	t.Helper()
	now := time.Now().UTC()
	staleSync := now.Add(-1 * time.Hour)
	tp := &TaskPR{
		TaskID:       taskID,
		WorkspaceID:  testWorkspaceID,
		RepositoryID: "repo-1",
		Owner:        feedbackSyncOwner,
		Repo:         feedbackSyncRepo,
		PRNumber:     number,
		PRURL:        "https://github.com/owner/repo/pull/2408",
		PRTitle:      "Collapse the dual writers",
		HeadBranch:   "feature/collapse",
		BaseBranch:   "main",
		State:        prStateOpen,
		CreatedAt:    now.Add(-3 * time.Hour),
		LastSyncedAt: &staleSync,
	}
	if mutate != nil {
		mutate(tp)
	}
	if err := store.CreateTaskPR(context.Background(), tp); err != nil {
		t.Fatalf("seed feedback task PR: %v", err)
	}
}

// addMergedFeedbackPR registers a merged PR the workspace's credentials can
// reach, so GetPRFeedbackForWorkspace resolves a client and returns live data.
func addMergedFeedbackPR(mockClient *MockClient, number int, mergedAt time.Time) {
	mockClient.AddRepos("test-user", []GitHubRepo{{
		Owner: feedbackSyncOwner, Name: feedbackSyncRepo,
		FullName: feedbackSyncOwner + "/" + feedbackSyncRepo,
	}})
	mockClient.AddPR(&PR{
		Number: number, Title: "Collapse the dual writers", State: prStateMerged,
		HTMLURL:    "https://github.com/owner/repo/pull/2408",
		HeadBranch: "feature/collapse", BaseBranch: "main", HeadSHA: feedbackSyncHead,
		RepoOwner: feedbackSyncOwner, RepoName: feedbackSyncRepo,
		CreatedAt: mergedAt.Add(-3 * time.Hour), UpdatedAt: mergedAt,
		MergedAt: &mergedAt, ClosedAt: &mergedAt,
	})
}

// The bug this collapses: GetPRFeedbackForWorkspace already holds an
// authoritative live *PR server-side, but used to return it without persisting.
// The browser then patched only its in-memory Zustand copy, so opening a PR's
// detail panel turned that tab purple while github_task_prs still said
// state=open — every other surface disagreed, and a reload snapped it back.
// Confirmed on a live instance for kdlbs/kandev#2408.
//
// This covers the stale-row direction (stored open, live merged). The inverse
// — stored merged, live read stale at open — is covered one layer down by
// TestSyncTaskPR_DoesNotRegressAMergedRow, deliberately: persistPRFeedbackState
// has no state logic of its own, it hands every row to the real SyncTaskPR, so
// asserting the guard at its own seam is where a break would actually surface.
func TestGetPRFeedbackForWorkspace_PersistsMergedStateOverStaleRow(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedFeedbackTaskPR(t, store, "task-1", 2408, nil)

	mergedAt := time.Now().UTC().Add(-10 * time.Minute)
	addMergedFeedbackPR(mockClient, 2408, mergedAt)

	// The published event is what converges every other surface, so subscribe
	// before the fetch and use it as the join.
	updated := subscribeTaskPRUpdated(t, svc)

	if _, err := svc.GetPRFeedbackForWorkspace(
		ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408,
	); err != nil {
		t.Fatalf("GetPRFeedbackForWorkspace: %v", err)
	}

	event := awaitTaskPRUpdated(t, updated)
	if event == nil || event.State != prStateMerged {
		t.Fatalf("published event state = %+v, want %q", event, prStateMerged)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 2408)
	if err != nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v", err)
	}
	if got == nil {
		t.Fatal("expected the task PR row to still exist")
	}
	if got.State != prStateMerged {
		t.Errorf("persisted state = %q, want %q", got.State, prStateMerged)
	}
	if got.MergedAt == nil {
		t.Error("persisted merged_at must be set once the feedback fetch observes the merge")
	}
}

// A feedback fetch computes checks and review counts (it makes the same
// GetPR + ListPRReviews + ListCheckRuns calls the REST status sync does) but
// never fetches review threads. SyncTaskPR's Populated flags must therefore
// carry unresolved_review_threads through untouched, while the two counts it
// did compute come from the real lists rather than being reset to zero.
func TestGetPRFeedbackForWorkspace_PartialSyncKeepsUncomputedAggregates(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedFeedbackTaskPR(t, store, "task-1", 2408, func(tp *TaskPR) {
		tp.ReviewCount = 3
		tp.ChecksTotal = 27
		tp.ChecksPassing = 22
		// Only the GraphQL path can compute this one.
		tp.UnresolvedReviewThreads = 4
	})

	mergedAt := time.Now().UTC().Add(-10 * time.Minute)
	addMergedFeedbackPR(mockClient, 2408, mergedAt)
	mockClient.AddReviews(feedbackSyncOwner, feedbackSyncRepo, 2408, []PRReview{
		{ID: 1, Author: "octocat", State: reviewStateApproved, CreatedAt: mergedAt},
	})
	mockClient.AddCheckRuns(feedbackSyncOwner, feedbackSyncRepo, feedbackSyncHead, []CheckRun{
		{Name: "build", Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
		{Name: "lint", Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
	})

	updated := subscribeTaskPRUpdated(t, svc)
	if _, err := svc.GetPRFeedbackForWorkspace(
		ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408,
	); err != nil {
		t.Fatalf("GetPRFeedbackForWorkspace: %v", err)
	}
	awaitTaskPRUpdated(t, updated)

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 2408)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v (row %+v)", err, got)
	}
	if got.UnresolvedReviewThreads != 4 {
		t.Errorf("unresolved_review_threads = %d, want the stored 4 preserved — "+
			"the feedback path never fetches review threads", got.UnresolvedReviewThreads)
	}
	// Counted from the real lists, so neither is allowed to land on zero.
	if got.ReviewCount != 1 {
		t.Errorf("review_count = %d, want 1 approving reviewer", got.ReviewCount)
	}
	if got.ChecksTotal != 2 || got.ChecksPassing != 2 {
		t.Errorf("checks = %d/%d, want 2/2", got.ChecksPassing, got.ChecksTotal)
	}
}

// The feedback path is TTL-cached and singleflight-coalesced; the persist must
// ride the upstream fetch, not the read. Otherwise every render of an open
// detail panel writes the row and republishes an event, and the WS handler
// applying that event re-renders the panel — a write amplifier on a read path.
func TestGetPRFeedbackForWorkspace_CachedResponseDoesNotRewriteRow(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedFeedbackTaskPR(t, store, "task-1", 2408, nil)

	mergedAt := time.Now().UTC().Add(-10 * time.Minute)
	addMergedFeedbackPR(mockClient, 2408, mergedAt)

	updated := subscribeTaskPRUpdated(t, svc)
	for i := range 3 {
		if _, err := svc.GetPRFeedbackForWorkspace(
			ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408,
		); err != nil {
			t.Fatalf("GetPRFeedbackForWorkspace call %d: %v", i, err)
		}
	}
	awaitTaskPRUpdated(t, updated)

	first, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 2408)
	if err != nil || first == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v (row %+v)", err, first)
	}
	writtenAt := first.LastSyncedAt
	if writtenAt == nil {
		t.Fatal("expected the first fetch to stamp last_synced_at")
	}

	// Cache hits must not touch the row at all.
	if _, err := svc.GetPRFeedbackForWorkspace(
		ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408,
	); err != nil {
		t.Fatalf("GetPRFeedbackForWorkspace (cached): %v", err)
	}
	after, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 2408)
	if err != nil || after == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v (row %+v)", err, after)
	}
	if !timeEqual(after.LastSyncedAt, writtenAt) {
		t.Errorf("cached feedback rewrote the row: last_synced_at %v -> %v", writtenAt, after.LastSyncedAt)
	}
	// ...and must not republish either, or the WS handler applying the event
	// re-renders the panel that triggered it. MemoryEventBus delivers to
	// subscribers synchronously (see MemoryEventBus.Publish), so any event the
	// call above caused is already in the channel — a non-blocking read is
	// exact here, with no sleep to go flaky under load.
	//
	// last_synced_at above is the assertion that actually catches a persist
	// moved outside the fetch closure; this one is defence in depth. A
	// redundant sync writes the row but re-derives identical field values, so
	// SyncTaskPR's `changed` guard already suppresses the event — this pins
	// that no-WS-traffic-on-a-cache-hit property against a future change that
	// relaxes that guard.
	select {
	case extra := <-updated:
		t.Errorf("cached feedback republished github.task_pr.updated: %+v", extra)
	default:
	}
}

// Rows belonging to another workspace must stay untouched: the feedback read
// was authorized against one workspace's credentials only, so the persist
// cannot reach past that boundary just because a PR number matches.
func TestGetPRFeedbackForWorkspace_DoesNotWriteOtherWorkspacesRows(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	if _, err := store.db.Exec(
		`INSERT INTO tasks (id, workspace_id, archived_at) VALUES (?, ?, NULL)`,
		"task-other", "ws-other"); err != nil {
		t.Fatalf("seed foreign task: %v", err)
	}
	seedFeedbackTaskPR(t, store, "task-1", 2408, nil)
	seedFeedbackTaskPR(t, store, "task-other", 2408, func(tp *TaskPR) {
		tp.WorkspaceID = "ws-other"
		tp.RepositoryID = "repo-other"
	})

	mergedAt := time.Now().UTC().Add(-10 * time.Minute)
	addMergedFeedbackPR(mockClient, 2408, mergedAt)

	updated := subscribeTaskPRUpdated(t, svc)
	if _, err := svc.GetPRFeedbackForWorkspace(
		ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408,
	); err != nil {
		t.Fatalf("GetPRFeedbackForWorkspace: %v", err)
	}
	awaitTaskPRUpdated(t, updated)

	foreign, err := store.GetTaskPRByRepoAndNumber(ctx, "task-other", "repo-other", 2408)
	if err != nil || foreign == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v (row %+v)", err, foreign)
	}
	if foreign.State != prStateOpen {
		t.Errorf("foreign-workspace row state = %q, want it left at %q", foreign.State, prStateOpen)
	}
}

// A failing store must not turn a working PR detail panel into an error: this
// is a read endpoint that gained a write, so the write is best-effort.
func TestGetPRFeedbackForWorkspace_PersistFailureDoesNotFailTheRead(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	seedFeedbackTaskPR(t, store, "task-1", 2408, nil)
	addMergedFeedbackPR(mockClient, 2408, time.Now().UTC().Add(-10*time.Minute))

	// Drop the table the persist path reads to force a store error underneath it.
	if _, err := store.db.Exec(`DROP TABLE tasks`); err != nil {
		t.Fatalf("drop tasks table: %v", err)
	}

	feedback, err := svc.GetPRFeedbackForWorkspace(
		ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408,
	)
	if err != nil {
		t.Fatalf("persist failure must not fail the feedback read: %v", err)
	}
	if feedback == nil || feedback.PR == nil || feedback.PR.State != prStateMerged {
		t.Fatalf("expected the live feedback to still be returned, got %+v", feedback)
	}
}

// SyncTaskPR is now the only writer, so the one invariant the deleted
// client-side hook was protecting has to live here: GitHub cannot un-merge a
// PR, so a stale live read reporting "open" for a merged row must not regress
// it. Only "merged" is terminal — see the reopen case below.
func TestSyncTaskPR_DoesNotRegressAMergedRow(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	mergedAt := time.Now().UTC().Add(-1 * time.Hour)
	seedFeedbackTaskPR(t, store, "task-1", 2408, func(tp *TaskPR) {
		tp.State = prStateMerged
		tp.MergedAt = &mergedAt
		tp.ClosedAt = &mergedAt
	})

	// A stale read — an eventually-consistent REST reply, or a fetch that
	// started before another writer observed the merge.
	stale := &PRStatus{PR: &PR{
		Number: 2408, State: prStateOpen, Title: "Collapse the dual writers",
		RepoOwner: feedbackSyncOwner, RepoName: feedbackSyncRepo, BaseBranch: "main",
	}}
	if err := svc.SyncTaskPR(ctx, "task-1", stale); err != nil {
		t.Fatalf("SyncTaskPR: %v", err)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 2408)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v (row %+v)", err, got)
	}
	if got.State != prStateMerged {
		t.Errorf("state = %q, want the terminal %q preserved", got.State, prStateMerged)
	}
	if got.MergedAt == nil {
		t.Error("merged_at must survive a stale non-merged read")
	}
}

// The counterpart: closed -> open is a real GitHub transition (a PR can be
// reopened), so the guard must be merged-only. A blanket merged > closed > open
// rank — what the frontend hook applied — would pin every reopened PR closed.
func TestSyncTaskPR_AllowsReopeningAClosedRow(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", false)
	closedAt := time.Now().UTC().Add(-1 * time.Hour)
	seedFeedbackTaskPR(t, store, "task-1", 2408, func(tp *TaskPR) {
		tp.State = prStateClosed
		tp.ClosedAt = &closedAt
	})

	reopened := &PRStatus{PR: &PR{
		Number: 2408, State: prStateOpen, Title: "Collapse the dual writers",
		RepoOwner: feedbackSyncOwner, RepoName: feedbackSyncRepo, BaseBranch: "main",
	}}
	if err := svc.SyncTaskPR(ctx, "task-1", reopened); err != nil {
		t.Fatalf("SyncTaskPR: %v", err)
	}

	got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 2408)
	if err != nil || got == nil {
		t.Fatalf("GetTaskPRByRepoAndNumber: %v (row %+v)", err, got)
	}
	if got.State != prStateOpen {
		t.Errorf("state = %q, want %q — reopening must not be treated as a regression", got.State, prStateOpen)
	}
	if got.ClosedAt != nil {
		t.Error("closed_at must clear when the PR reopens")
	}
}
