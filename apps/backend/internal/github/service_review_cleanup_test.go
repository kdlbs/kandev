package github

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type countingCleanupClient struct {
	*MockClient
	prCalls     int
	reviewCalls int
}

func (c *countingCleanupClient) GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error) {
	return c.MockClient.GetPRFeedback(ctx, owner, repo, number)
}

func (c *countingCleanupClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	c.prCalls++
	return c.MockClient.GetPR(ctx, owner, repo, number)
}

func (c *countingCleanupClient) ListPRReviews(ctx context.Context, owner, repo string, number int) ([]PRReview, error) {
	c.reviewCalls++
	return c.MockClient.ListPRReviews(ctx, owner, repo, number)
}

func TestCleanupMergedReviewTasksSkipsHistoricalTasks(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	client := &countingCleanupClient{MockClient: mockClient}
	configureTestWorkspaceAuth(t, svc, client, "ws-1")

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	seedTask(t, store, "active-review", false)
	seedTask(t, store, "archived-review", true)

	rows := []*ReviewPRTask{
		{ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 1, TaskID: "active-review"},
		{ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 2, TaskID: "archived-review"},
		{ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 3, TaskID: "missing-review"},
		{ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 4},
		{ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 5},
	}
	for _, row := range rows {
		if err := store.CreateReviewPRTask(ctx, row); err != nil {
			t.Fatalf("CreateReviewPRTask #%d: %v", row.PRNumber, err)
		}
	}
	mockClient.AddPR(&PR{Number: 1, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})
	mockClient.AddPR(&PR{Number: 2, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})
	mockClient.AddPR(&PR{Number: 3, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})
	mockClient.AddPR(&PR{Number: 4, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})
	mockClient.AddPR(&PR{Number: 5, State: prStateOpen, RepoOwner: "acme", RepoName: "widget"})

	recorder := &recordingTaskDeleter{}
	svc.SetTaskDeleter(recorder)

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil {
		t.Fatalf("CleanupMergedReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1 active task", deleted)
	}
	if len(recorder.calls) != 1 || recorder.calls[0] != "active-review" {
		t.Fatalf("DeleteTask calls = %v, want [active-review]", recorder.calls)
	}
	if client.prCalls != 3 {
		t.Fatalf("GetPR calls = %d, want 3 eligible rows only", client.prCalls)
	}
	if client.reviewCalls != 1 {
		t.Fatalf("ListPRReviews calls = %d, want 1 for the open eligible row", client.reviewCalls)
	}

	remaining, err := store.ListReviewPRTaskIDsByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTaskIDsByWatch: %v", err)
	}
	got := make(map[string]bool, len(remaining))
	for _, taskID := range remaining {
		got[taskID] = true
	}
	want := map[string]bool{"archived-review": true, "missing-review": true, "": true}
	if len(got) != len(want) || !got["archived-review"] || !got["missing-review"] || !got[""] {
		t.Fatalf("remaining task IDs = %#v, want historical rows and open empty reservation", got)
	}
}

type minimalCleanupClient struct {
	NoopClient
	prs              map[int]*PR
	reviews          map[int][]PRReview
	user             string
	getPRCalls       int
	listReviewCalls  int
	userCalls        int
	feedbackCalls    int
	commentCalls     int
	checkCalls       int
	workflowRunCalls int
	workflowJobCalls int
}

func (c *minimalCleanupClient) GetPR(_ context.Context, owner, repo string, number int) (*PR, error) {
	c.getPRCalls++
	pr, ok := c.prs[number]
	if !ok {
		return nil, errors.New("unexpected PR lookup")
	}
	copy := *pr
	copy.RepoOwner = owner
	copy.RepoName = repo
	return &copy, nil
}

func (c *minimalCleanupClient) ListPRReviews(_ context.Context, _, _ string, number int) ([]PRReview, error) {
	c.listReviewCalls++
	return append([]PRReview(nil), c.reviews[number]...), nil
}

func (c *minimalCleanupClient) GetAuthenticatedUser(context.Context) (string, error) {
	c.userCalls++
	return c.user, nil
}

func (c *minimalCleanupClient) GetPRFeedback(context.Context, string, string, int) (*PRFeedback, error) {
	c.feedbackCalls++
	return nil, errors.New("cleanup must not fetch PR feedback")
}

func (c *minimalCleanupClient) ListPRComments(context.Context, string, string, int, *time.Time) ([]PRComment, error) {
	c.commentCalls++
	return nil, errors.New("cleanup must not fetch PR comments")
}

func (c *minimalCleanupClient) ListCheckRuns(context.Context, string, string, string) ([]CheckRun, error) {
	c.checkCalls++
	return nil, errors.New("cleanup must not fetch check runs")
}

func (c *minimalCleanupClient) ListWorkflowRuns(context.Context, string, string, string) ([]WorkflowRun, error) {
	c.workflowRunCalls++
	return nil, errors.New("cleanup must not fetch workflow runs")
}

func (c *minimalCleanupClient) ListWorkflowRunJobs(context.Context, string, string, int64, int) ([]WorkflowJob, error) {
	c.workflowJobCalls++
	return nil, errors.New("cleanup must not fetch workflow jobs")
}

func TestCleanupUsesMinimalPRReads(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	client := &minimalCleanupClient{
		prs: map[int]*PR{
			1: {Number: 1, State: prStateMerged},
			2: {Number: 2, State: prStateClosed},
			3: {Number: 3, State: prStateOpen},
			4: {Number: 4, State: prStateOpen},
		},
		reviews: map[int][]PRReview{
			3: {{State: "APPROVED", Author: "alice"}},
			4: {{State: "COMMENTED", Author: "reviewer"}},
		},
		user: "alice",
	}
	configureTestWorkspaceAuth(t, svc, client, "ws-1")
	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	for number := 1; number <= 4; number++ {
		taskID := fmt.Sprintf("cleanup-task-%d", number)
		seedTask(t, store, taskID, false)
		if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
			ReviewWatchID: watch.ID,
			RepoOwner:     "acme",
			RepoName:      "widget",
			PRNumber:      number,
			TaskID:        taskID,
		}); err != nil {
			t.Fatalf("CreateReviewPRTask #%d: %v", number, err)
		}
	}
	recorder := &recordingTaskDeleter{}
	svc.SetTaskDeleter(recorder)

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil || deleted != 3 {
		t.Fatalf("cleanup = %d, %v; want three terminal or approved tasks", deleted, err)
	}
	if client.getPRCalls != 4 {
		t.Fatalf("GetPR calls = %d, want 4", client.getPRCalls)
	}
	if client.listReviewCalls != 2 {
		t.Fatalf("ListPRReviews calls = %d, want 2 for open PRs", client.listReviewCalls)
	}
	if client.userCalls != 1 {
		t.Fatalf("GetAuthenticatedUser calls = %d, want 1 for the approval candidate", client.userCalls)
	}
	if client.feedbackCalls != 0 || client.commentCalls != 0 || client.checkCalls != 0 || client.workflowRunCalls != 0 || client.workflowJobCalls != 0 {
		t.Fatalf("unexpected cleanup reads: feedback=%d comments=%d checks=%d workflow_runs=%d workflow_jobs=%d", client.feedbackCalls, client.commentCalls, client.checkCalls, client.workflowRunCalls, client.workflowJobCalls)
	}
}

// stubTaskDeleter implements TaskDeleter for testing.
type stubTaskDeleter struct {
	err error
}

func (s *stubTaskDeleter) DeleteTask(_ context.Context, _ string) error {
	return s.err
}

// TestCleanupMergedReviewTasks_TaskAlreadyDeleted verifies that a dedup row for
// a missing task is ignored without consuming provider quota.
func TestCleanupMergedReviewTasks_TaskAlreadyDeleted(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Create a review watch.
	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	// Create a dedup record pointing to an already-deleted task.
	taskID := "task-already-gone"
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      42,
		PRURL:         "https://github.com/acme/widget/pull/42",
		TaskID:        taskID,
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}

	// Mock: PR is merged so shouldDeleteReviewTask returns true.
	mockClient.AddPR(&PR{
		Number:    42,
		State:     prStateMerged,
		RepoOwner: "acme",
		RepoName:  "widget",
	})

	// A missing task is not a cleanup candidate. The task deleter remains wired
	// to prove the cleanup path does not try to delete it.
	svc.SetTaskDeleter(&stubTaskDeleter{
		err: fmt.Errorf("%w: %s", ErrTaskNotFound, taskID),
	})

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil {
		t.Fatalf("CleanupMergedReviewTasks returned error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted for a missing task, got %d", deleted)
	}

	// The historical dedup record remains available for deduplication, but is
	// excluded from cleanup candidate queries.
	remaining, err := store.ListReviewPRTaskIDsByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTaskIDsByWatch: %v", err)
	}
	if len(remaining) != 1 || remaining[0] != taskID {
		t.Errorf("remaining task IDs = %v, want [%s]", remaining, taskID)
	}
}

type callbackTaskDeleter func(context.Context, string) error

func (f callbackTaskDeleter) DeleteTask(ctx context.Context, taskID string) error {
	return f(ctx, taskID)
}

func TestCleanupMergedReviewTasks_TaskDeletedAfterSelection(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	seedTask(t, store, "task-race", false)
	if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      43,
		TaskID:        "task-race",
	}); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}
	mockClient.AddPR(&PR{Number: 43, State: prStateMerged, RepoOwner: "acme", RepoName: "widget"})
	svc.SetTaskDeleter(callbackTaskDeleter(func(ctx context.Context, taskID string) error {
		if _, err := store.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, taskID); err != nil {
			t.Fatalf("delete selected task: %v", err)
		}
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}))

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil {
		t.Fatalf("CleanupMergedReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1 after selected task disappears", deleted)
	}
	remaining, err := store.ListReviewPRTaskIDsByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTaskIDsByWatch: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining task IDs = %v, want none", remaining)
	}
}

// Regression: when a task already has a TaskPR row pointing to PR #1 and
// AssociatePRWithTask is called with a different PR #2 (e.g. first PR
// closed, new PR opened — or a multi-branch task gaining a second PR on a
// different branch), the new row must be inserted as a sibling without
// touching the existing #1 row. Multi-branch tasks rely on this so two
// PRs on the same (task, repo) coexist; the old "delete-then-insert"
// behavior collapsed multi-branch tasks down to one PR row.
func TestAssociatePRWithTask_AddsSecondPRAsSibling(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()

	// Seed an existing association for PR #1.
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:     "t1",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   1,
		PRURL:      "https://github.com/owner/repo/pull/1",
		PRTitle:    "First",
		HeadBranch: "feat-a",
		BaseBranch: "main",
		State:      "closed",
	}); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}

	// Associate a new PR #2 on a different branch.
	newPR := &PR{
		Number:      2,
		Title:       "Second",
		HTMLURL:     "https://github.com/owner/repo/pull/2",
		HeadBranch:  "feat-b",
		BaseBranch:  "main",
		State:       "open",
		RepoOwner:   "owner",
		RepoName:    "repo",
		AuthorLogin: "alice",
	}
	tp, err := svc.AssociatePRWithTask(ctx, "t1", "", newPR)
	if err != nil {
		t.Fatalf("AssociatePRWithTask: %v", err)
	}
	if tp.PRNumber != 2 {
		t.Errorf("returned TaskPR.PRNumber=%d, want 2", tp.PRNumber)
	}

	all, err := store.ListTaskPRsByTask(ctx, "t1")
	if err != nil {
		t.Fatalf("ListTaskPRsByTask: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 PR rows after associating second PR, got %d", len(all))
	}
	nums := map[int]bool{}
	for _, r := range all {
		nums[r.PRNumber] = true
	}
	if !nums[1] || !nums[2] {
		t.Errorf("missing expected PR numbers: %v", nums)
	}
}
