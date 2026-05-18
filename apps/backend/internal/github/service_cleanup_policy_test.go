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
