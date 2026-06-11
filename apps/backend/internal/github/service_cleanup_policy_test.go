package github

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	return log
}

// recordingSessionChecker satisfies TaskSessionChecker for tests and lets the
// caller dial in "did the user type something" without spinning up a real
// task repository.
type recordingSessionChecker struct {
	hasUserMsg bool
	err        error
	calls      int
}

func (s *recordingSessionChecker) HasUserAuthoredMessage(_ context.Context, _ string) (bool, error) {
	s.calls++
	return s.hasUserMsg, s.err
}

// prFeedbackStub returns a canned PRFeedback for shouldDeleteReviewTask tests.
// Unrelated client methods come from NoopClient.
type prFeedbackStub struct {
	NoopClient
	state string
	err   error
}

func (c *prFeedbackStub) GetPRFeedback(_ context.Context, _, _ string, _ int) (*PRFeedback, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &PRFeedback{PR: &PR{State: c.state}}, nil
}

func reviewTaskFixture() *ReviewPRTask {
	return &ReviewPRTask{
		ID:            "rpt-1",
		ReviewWatchID: "rw-1",
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      99,
		TaskID:        "task-99",
	}
}

func TestShouldDeleteReviewTask_AutoPolicy_NoUserMessages_Deletes(t *testing.T) {
	log := testLogger(t)
	checker := &recordingSessionChecker{hasUserMsg: false}
	svc := newCleanupTestService(&prFeedbackStub{state: prStateMerged}, log, checker)

	del, reason := svc.shouldDeleteReviewTask(context.Background(), reviewTaskFixture(), CleanupPolicyAuto)
	if !del {
		t.Fatalf("expected delete=true under auto with no user messages")
	}
	if reason != "pr_merged_or_closed" {
		t.Fatalf("reason=%q, want pr_merged_or_closed", reason)
	}
	if checker.calls != 1 {
		t.Fatalf("HasUserAuthoredMessage calls=%d, want 1", checker.calls)
	}
}

func TestShouldDeleteReviewTask_AutoPolicy_UserMessage_Preserves(t *testing.T) {
	log := testLogger(t)
	checker := &recordingSessionChecker{hasUserMsg: true}
	svc := newCleanupTestService(&prFeedbackStub{state: prStateClosed}, log, checker)

	del, _ := svc.shouldDeleteReviewTask(context.Background(), reviewTaskFixture(), CleanupPolicyAuto)
	if del {
		t.Fatalf("expected delete=false when user authored a message under auto")
	}
}

func TestShouldDeleteReviewTask_AlwaysPolicy_IgnoresUserMessages(t *testing.T) {
	log := testLogger(t)
	checker := &recordingSessionChecker{hasUserMsg: true}
	svc := newCleanupTestService(&prFeedbackStub{state: prStateMerged}, log, checker)

	del, reason := svc.shouldDeleteReviewTask(context.Background(), reviewTaskFixture(), CleanupPolicyAlways)
	if !del {
		t.Fatalf("expected delete=true under always-policy even with user messages")
	}
	if reason != "pr_merged_or_closed" {
		t.Fatalf("reason=%q, want pr_merged_or_closed", reason)
	}
	if checker.calls != 0 {
		t.Fatalf("HasUserAuthoredMessage calls=%d, want 0 (always-policy short-circuits)", checker.calls)
	}
}

func TestShouldDeleteReviewTask_NeverPolicy_KeepsEverything(t *testing.T) {
	log := testLogger(t)
	checker := &recordingSessionChecker{hasUserMsg: false}
	svc := newCleanupTestService(&prFeedbackStub{state: prStateMerged}, log, checker)

	del, _ := svc.shouldDeleteReviewTask(context.Background(), reviewTaskFixture(), CleanupPolicyNever)
	if del {
		t.Fatalf("expected delete=false under never-policy")
	}
	// Never-policy must skip the upstream call too — no point burning quota.
	if checker.calls != 0 {
		t.Fatalf("HasUserAuthoredMessage calls=%d, want 0 (never-policy short-circuits)", checker.calls)
	}
}

func TestShouldDeleteReviewTask_OpenPR_KeepsTask(t *testing.T) {
	log := testLogger(t)
	checker := &recordingSessionChecker{hasUserMsg: false}
	svc := newCleanupTestService(&prFeedbackStub{state: "open"}, log, checker)

	del, _ := svc.shouldDeleteReviewTask(context.Background(), reviewTaskFixture(), CleanupPolicyAuto)
	if del {
		t.Fatalf("expected delete=false for open PR")
	}
}

func TestShouldDeleteReviewTask_GitHubError_TracksFailureCount(t *testing.T) {
	log := testLogger(t)
	svc := newCleanupTestService(&prFeedbackStub{err: fmt.Errorf("rate limit hit")}, log, &recordingSessionChecker{})

	rpt := reviewTaskFixture()
	for i := 0; i < cleanupFetchFailureThreshold+1; i++ {
		if del, _ := svc.shouldDeleteReviewTask(context.Background(), rpt, CleanupPolicyAuto); del {
			t.Fatalf("iteration %d: expected delete=false under fetch error", i)
		}
	}
	// The counter is keyed per-row; one error → one increment.
	key := reviewFailureKey(rpt)
	svc.cleanupFailureMu.Lock()
	got := svc.cleanupFailureCounts[key]
	svc.cleanupFailureMu.Unlock()
	if got != cleanupFetchFailureThreshold+1 {
		t.Fatalf("failure count = %d, want %d", got, cleanupFetchFailureThreshold+1)
	}
}

func TestShouldDeleteReviewTask_GitHubErrorThenSuccess_ResetsCounter(t *testing.T) {
	log := testLogger(t)
	client := &switchingFeedbackClient{err: fmt.Errorf("transient")}
	svc := newCleanupTestService(client, log, &recordingSessionChecker{})

	rpt := reviewTaskFixture()
	// First call fails — only the side effect (counter inc) matters here.
	_, _ = svc.shouldDeleteReviewTask(context.Background(), rpt, CleanupPolicyAuto)
	// Now switch the client to a healthy response — the next call must reset.
	client.err = nil
	client.state = prStateMerged
	_, _ = svc.shouldDeleteReviewTask(context.Background(), rpt, CleanupPolicyAuto)
	key := reviewFailureKey(rpt)
	svc.cleanupFailureMu.Lock()
	_, present := svc.cleanupFailureCounts[key]
	svc.cleanupFailureMu.Unlock()
	if present {
		t.Fatalf("failure counter should be cleared after a successful fetch")
	}
}

func TestCleanupAllOrphanedReviewTasks_DeletedWatchOrphans(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Watch + dedup row + merged PR.
	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      77,
		PRURL:         "https://github.com/acme/widget/pull/77",
		TaskID:        "task-77",
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}
	mockClient.AddPR(&PR{Number: 77, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})

	// Simulate the watch having been deleted out from under the dedup row by
	// removing only the watch (bypassing the cascade so the orphan survives).
	if _, err := store.db.Exec(`DELETE FROM github_review_watches WHERE id = ?`, watch.ID); err != nil {
		t.Fatalf("manual watch delete: %v", err)
	}

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedReviewTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1 (orphan with missing watch should fall back to auto policy and reap)", deleted)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "task-77" {
		t.Fatalf("DeleteTask calls=%v, want [task-77]", rec.calls)
	}
}

func TestCleanupAllOrphanedReviewTasks_DisabledWatch_StillCleansUp(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: false}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      88,
		TaskID:        "task-88",
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}
	mockClient.AddPR(&PR{Number: 88, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedReviewTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1 (disabled watch's PR task should still be reaped by global sweep)", deleted)
	}
}

// Regression for cubic P1: when GetReviewWatch fails for a watch, the row
// MUST be skipped this cycle. Treating it as orphan would fail-open under
// the auto policy, reaping enabled-watch rows (potentially even rows whose
// real watch is configured as `never`) on a transient DB hiccup.
func TestCleanupAllOrphanedReviewTasks_UnknownWatchSkipped(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyNever}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      222,
		TaskID:        "task-222",
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}
	mockClient.AddPR(&PR{Number: 222, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})

	// Wedge the watch row so GetReviewWatch errors. Setting a bad enum value
	// would still parse; instead, drop the table briefly. We simulate the
	// store error by closing the read DB connection.
	if _, err := store.db.Exec(`ALTER TABLE github_review_watches RENAME TO github_review_watches_x`); err != nil {
		t.Fatalf("rename watches table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(`ALTER TABLE github_review_watches_x RENAME TO github_review_watches`)
	})

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedReviewTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedReviewTasks: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 (row whose watch fetch errored must be skipped, not fail-open as orphan)", deleted)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("DeleteTask called %d times, want 0", len(rec.calls))
	}
}

// Regression for greptile P2: the global sweep must not re-hit GitHub for
// dedup rows the per-watch loop already processed in the same cycle.
// CleanupAllOrphanedReviewTasks skips rows whose watch is currently
// enabled — the per-watch loop is responsible for those.
func TestCleanupAllOrphanedReviewTasks_SkipsEnabledWatchRows(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      111,
		TaskID:        "task-111",
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}
	// PR is merged — without the skip, the orphan sweep would happily
	// delete it on top of the per-watch loop.
	mockClient.AddPR(&PR{Number: 111, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedReviewTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedReviewTasks: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 (enabled-watch rows must be skipped by the orphan sweep)", deleted)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("DeleteTask called %d times, want 0", len(rec.calls))
	}

	// And CleanupAllReviewTasks (manual button path) reaps the same row.
	deleted, err = svc.CleanupAllReviewTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("manual sweep deleted=%d, want 1", deleted)
	}
}

func TestCleanupAllOrphanedReviewTasks_RespectsNeverPolicy(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyNever}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      55,
		TaskID:        "task-55",
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}
	mockClient.AddPR(&PR{Number: 55, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedReviewTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedReviewTasks: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 (never-policy must keep tasks even when PR merges)", deleted)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("DeleteTask calls=%v, want none", rec.calls)
	}
}

// Regression for claude blocker on c4d0a678: when ListEnabledReviewWatches
// returns an empty slice (user disabled or deleted every watch), the
// poller used to early-return BEFORE running the orphan sweep — so the
// sweep that exists precisely for this scenario never fired. Verify the
// poller's checkReviewWatches now runs the sweep through to completion
// and reaps the orphan task.
func TestCheckReviewWatches_RunsOrphanSweepWhenNoEnabledWatches(t *testing.T) {
	poller, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Create a watch + dedup row, then disable the watch so
	// ListEnabledReviewWatches returns empty. The dedup row + task survive.
	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: false}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      9001,
		TaskID:        "task-9001",
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}
	mockClient.AddPR(&PR{Number: 9001, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	poller.checkReviewWatches(ctx)

	if len(rec.calls) != 1 || rec.calls[0] != "task-9001" {
		t.Fatalf("DeleteTask calls=%v, want [task-9001] (orphan sweep must run even with zero enabled watches)", rec.calls)
	}
}

// Same regression on the issue side.
func TestCheckIssueWatches_RunsOrphanSweepWhenNoEnabledWatches(t *testing.T) {
	poller, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	svc.client = newIssueStateMockClient(map[int]string{9002: "closed"})

	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: false}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	_, _ = store.ReserveIssueWatchTask(ctx, watch.ID, "acme", "widget", 9002, "https://example/9002")
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", 9002, "task-9002"); err != nil {
		t.Fatalf("AssignIssueWatchTaskID: %v", err)
	}

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	poller.checkIssueWatches(ctx)

	if len(rec.calls) != 1 || rec.calls[0] != "task-9002" {
		t.Fatalf("DeleteTask calls=%v, want [task-9002] (orphan sweep must run even with zero enabled watches)", rec.calls)
	}
}

// --- Issue-side mirrors ---
// These tests mirror the review-side coverage above for the symmetric
// issue path. Without them a copy-paste divergence between
// cleanupAllReviewTasks and cleanupAllIssueTasks — wrong cache key,
// inverted predicate, forgotten unknown-watch guard — would only surface
// in a production incident.

// issueStateMockClient extends the default MockClient with per-issue state
// lookup. MockClient.GetIssueState always returns "open"; cleanup tests
// need to flip individual issues to "closed".
type issueStateMockClient struct {
	*MockClient
	states map[int]string // issue number -> state
}

func newIssueStateMockClient(states map[int]string) *issueStateMockClient {
	return &issueStateMockClient{MockClient: NewMockClient(), states: states}
}

func (c *issueStateMockClient) GetIssueState(_ context.Context, _, _ string, number int) (string, error) {
	if s, ok := c.states[number]; ok {
		return s, nil
	}
	return "open", nil
}

func TestCleanupAllOrphanedIssueTasks_DeletedWatchOrphans(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	svc.client = newIssueStateMockClient(map[int]string{77: "closed"})

	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	reserved, err := store.ReserveIssueWatchTask(ctx, watch.ID, "acme", "widget", 77, "https://example/77")
	if err != nil || !reserved {
		t.Fatalf("ReserveIssueWatchTask: ok=%v err=%v", reserved, err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", 77, "task-77"); err != nil {
		t.Fatalf("AssignIssueWatchTaskID: %v", err)
	}

	if _, err := store.db.Exec(`DELETE FROM github_issue_watches WHERE id = ?`, watch.ID); err != nil {
		t.Fatalf("manual watch delete: %v", err)
	}

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedIssueTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedIssueTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1 (orphan with missing watch should fall back to auto policy and reap)", deleted)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "task-77" {
		t.Fatalf("DeleteTask calls=%v, want [task-77]", rec.calls)
	}
}

func TestCleanupAllOrphanedIssueTasks_DisabledWatch_StillCleansUp(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	svc.client = newIssueStateMockClient(map[int]string{88: "closed"})

	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: false}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	_, _ = store.ReserveIssueWatchTask(ctx, watch.ID, "acme", "widget", 88, "https://example/88")
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", 88, "task-88"); err != nil {
		t.Fatalf("AssignIssueWatchTaskID: %v", err)
	}

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedIssueTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedIssueTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1 (disabled watch's issue task should still be reaped by global sweep)", deleted)
	}
}

func TestCleanupAllOrphanedIssueTasks_UnknownWatchSkipped(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	svc.client = newIssueStateMockClient(map[int]string{222: "closed"})

	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyNever}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	_, _ = store.ReserveIssueWatchTask(ctx, watch.ID, "acme", "widget", 222, "https://example/222")
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", 222, "task-222"); err != nil {
		t.Fatalf("AssignIssueWatchTaskID: %v", err)
	}

	if _, err := store.db.Exec(`ALTER TABLE github_issue_watches RENAME TO github_issue_watches_x`); err != nil {
		t.Fatalf("rename watches table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.Exec(`ALTER TABLE github_issue_watches_x RENAME TO github_issue_watches`)
	})

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedIssueTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedIssueTasks: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 (row whose watch fetch errored must be skipped, not fail-open as orphan)", deleted)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("DeleteTask called %d times, want 0", len(rec.calls))
	}
}

func TestCleanupAllOrphanedIssueTasks_SkipsEnabledWatchRows(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	svc.client = newIssueStateMockClient(map[int]string{111: "closed"})

	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	_, _ = store.ReserveIssueWatchTask(ctx, watch.ID, "acme", "widget", 111, "https://example/111")
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", 111, "task-111"); err != nil {
		t.Fatalf("AssignIssueWatchTaskID: %v", err)
	}

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedIssueTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedIssueTasks: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 (enabled-watch rows must be skipped by the orphan sweep)", deleted)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("DeleteTask called %d times, want 0", len(rec.calls))
	}

	// And CleanupAllIssueTasks (manual button path) reaps the same row.
	deleted, err = svc.CleanupAllIssueTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllIssueTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("manual sweep deleted=%d, want 1", deleted)
	}
}

func TestCleanupAllOrphanedIssueTasks_RespectsNeverPolicy(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	svc.client = newIssueStateMockClient(map[int]string{55: "closed"})

	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyNever}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	_, _ = store.ReserveIssueWatchTask(ctx, watch.ID, "acme", "widget", 55, "https://example/55")
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", 55, "task-55"); err != nil {
		t.Fatalf("AssignIssueWatchTaskID: %v", err)
	}

	rec := &recordingTaskDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllOrphanedIssueTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllOrphanedIssueTasks: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted=%d, want 0 (never-policy must keep tasks even when issue closes)", deleted)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("DeleteTask calls=%v, want none", rec.calls)
	}
}

func TestDeleteIssueWatch_CascadesDedupRows(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	for _, n := range []int{1, 2, 3} {
		_, _ = store.ReserveIssueWatchTask(ctx, watch.ID, "acme", "widget", n, fmt.Sprintf("https://example/%d", n))
		if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "acme", "widget", n, fmt.Sprintf("task-%d", n)); err != nil {
			t.Fatalf("AssignIssueWatchTaskID: %v", err)
		}
	}

	if err := store.DeleteIssueWatch(ctx, watch.ID); err != nil {
		t.Fatalf("DeleteIssueWatch: %v", err)
	}

	remaining, err := store.ListIssueWatchTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListIssueWatchTasksByWatch: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("dedup rows leaked after watch delete: %d remaining", len(remaining))
	}
}

func TestDeleteReviewWatch_CascadesDedupRows(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	for _, n := range []int{1, 2, 3} {
		rpt := &ReviewPRTask{
			ReviewWatchID: watch.ID,
			RepoOwner:     "acme",
			RepoName:      "widget",
			PRNumber:      n,
			TaskID:        fmt.Sprintf("task-%d", n),
		}
		if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
			t.Fatalf("CreateReviewPRTask: %v", err)
		}
	}

	if err := store.DeleteReviewWatch(ctx, watch.ID); err != nil {
		t.Fatalf("DeleteReviewWatch: %v", err)
	}

	remaining, err := store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("dedup rows leaked after watch delete: %d remaining", len(remaining))
	}
}

func TestDeleteReviewWatchesByWorkspace(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	mk := func(ws string) *ReviewWatch {
		w := &ReviewWatch{WorkspaceID: ws, Enabled: true}
		if err := store.CreateReviewWatch(ctx, w); err != nil {
			t.Fatalf("CreateReviewWatch: %v", err)
		}
		rpt := &ReviewPRTask{
			ReviewWatchID: w.ID,
			RepoOwner:     "acme",
			RepoName:      "widget",
			PRNumber:      1,
			TaskID:        "task-" + w.ID,
		}
		if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
			t.Fatalf("CreateReviewPRTask: %v", err)
		}
		return w
	}
	// Two watches in ws-1 (to be cleared) and one in ws-2 that must survive.
	w1 := mk("ws-1")
	w2 := mk("ws-1")
	wOther := mk("ws-2")

	deleted, err := svc.DeleteReviewWatchesByWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("DeleteReviewWatchesByWorkspace: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}

	ws1, err := store.ListReviewWatches(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListReviewWatches ws-1: %v", err)
	}
	if len(ws1) != 0 {
		t.Fatalf("ws-1 watches remain: %d", len(ws1))
	}
	ws2, err := store.ListReviewWatches(ctx, "ws-2")
	if err != nil {
		t.Fatalf("ListReviewWatches ws-2: %v", err)
	}
	if len(ws2) != 1 {
		t.Fatalf("ws-2 watches = %d, want 1 (unaffected)", len(ws2))
	}

	// Dedup rows for the deleted watches are gone; the survivor's remain.
	for _, w := range []*ReviewWatch{w1, w2} {
		rows, err := store.ListReviewPRTasksByWatch(ctx, w.ID)
		if err != nil {
			t.Fatalf("ListReviewPRTasksByWatch: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("dedup rows leaked for deleted watch %s: %d", w.ID, len(rows))
		}
	}
	survivor, err := store.ListReviewPRTasksByWatch(ctx, wOther.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch survivor: %v", err)
	}
	if len(survivor) != 1 {
		t.Fatalf("survivor dedup rows = %d, want 1", len(survivor))
	}
}

func TestNormalizeCleanupPolicy(t *testing.T) {
	if got := NormalizeCleanupPolicy(""); got != CleanupPolicyAuto {
		t.Fatalf("empty → %q, want auto", got)
	}
	if got := NormalizeCleanupPolicy(CleanupPolicyAlways); got != CleanupPolicyAlways {
		t.Fatalf("always → %q, want always", got)
	}
}

func TestIsValidCleanupPolicy(t *testing.T) {
	for _, ok := range []string{"", CleanupPolicyAuto, CleanupPolicyAlways, CleanupPolicyNever} {
		if !IsValidCleanupPolicy(ok) {
			t.Errorf("expected %q valid", ok)
		}
	}
	for _, bad := range []string{"sometimes", "AUTO", "1"} {
		if IsValidCleanupPolicy(bad) {
			t.Errorf("expected %q invalid", bad)
		}
	}
}

// switchingFeedbackClient flips between failure and success on each call so
// tests can simulate "transient outage clears".
type switchingFeedbackClient struct {
	NoopClient
	state string
	err   error
}

func (c *switchingFeedbackClient) GetPRFeedback(_ context.Context, _, _ string, _ int) (*PRFeedback, error) {
	if c.err != nil {
		return nil, c.err
	}
	return &PRFeedback{PR: &PR{State: c.state}}, nil
}
