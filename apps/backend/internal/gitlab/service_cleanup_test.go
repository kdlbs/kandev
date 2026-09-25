package gitlab

import (
	"context"
	"testing"
)

type countingGitLabCleanupClient struct {
	*MockClient
	mrStatusCalls   int
	issueStateCalls int
}

func (c *countingGitLabCleanupClient) GetMRStatus(ctx context.Context, projectPath string, iid int) (*MRStatus, error) {
	c.mrStatusCalls++
	return c.MockClient.GetMRStatus(ctx, projectPath, iid)
}

func (c *countingGitLabCleanupClient) GetIssueState(ctx context.Context, projectPath string, iid int) (string, error) {
	c.issueStateCalls++
	return c.MockClient.GetIssueState(ctx, projectPath, iid)
}

type cleanupTaskRecorder struct {
	taskIDs []string
}

func (r *cleanupTaskRecorder) DeleteTask(_ context.Context, taskID string) error {
	r.taskIDs = append(r.taskIDs, taskID)
	return nil
}

func TestCleanupSkipsHistoricalTasks(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")

	client := &countingGitLabCleanupClient{MockClient: NewMockClient(DefaultHost)}
	client.SeedMR("team/review", &MR{IID: 1, State: gitlabStateMerged})
	client.SeedIssue("team/issue", &Issue{IID: 1, State: gitlabStateClosed})
	service := NewService(DefaultHost, client, AuthMethodNone, nil, newTestLogger(t))
	service.SetStore(store)
	recorder := &cleanupTaskRecorder{}
	service.SetTaskDeleter(recorder)

	reviewWatch := &ReviewWatch{ID: "review-cleanup-service", WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateReviewWatch(t.Context(), reviewWatch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	issueWatch := &IssueWatch{ID: "issue-cleanup-service", WorkspaceID: "ws-1", CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateIssueWatch(t.Context(), issueWatch); err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	for _, taskID := range []string{"review-active", "review-archived", "issue-active", "issue-archived"} {
		seedTask(t, store, taskID, "ws-1")
	}
	for _, taskID := range []string{"review-archived", "issue-archived"} {
		if _, err := store.db.Exec(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
			t.Fatalf("archive task %s: %v", taskID, err)
		}
	}

	reserveReview := func(iid int, taskID string) {
		t.Helper()
		reserved, err := store.ReserveReviewMRTask(t.Context(), reviewWatch.ID, "team/review", iid, "https://gitlab.example/mr")
		if err != nil || !reserved {
			t.Fatalf("reserve review %d: reserved=%v err=%v", iid, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignReviewMRTaskID(t.Context(), reviewWatch.ID, "team/review", iid, taskID); err != nil {
				t.Fatalf("assign review %d: %v", iid, err)
			}
		}
	}
	reserveIssue := func(iid int, taskID string) {
		t.Helper()
		reserved, err := store.ReserveIssueWatchTask(t.Context(), issueWatch.ID, "team/issue", iid, "https://gitlab.example/issue")
		if err != nil || !reserved {
			t.Fatalf("reserve issue %d: reserved=%v err=%v", iid, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignIssueWatchTaskID(t.Context(), issueWatch.ID, "team/issue", iid, taskID); err != nil {
				t.Fatalf("assign issue %d: %v", iid, err)
			}
		}
	}
	reserveReview(1, "review-active")
	reserveReview(2, "review-archived")
	reserveReview(3, "review-missing")
	reserveReview(4, "")
	reserveIssue(1, "issue-active")
	reserveIssue(2, "issue-archived")
	reserveIssue(3, "issue-missing")
	reserveIssue(4, "")

	if deleted, err := service.CleanupAllReviewTasks(t.Context()); err != nil || deleted != 1 {
		t.Fatalf("review cleanup = %d, %v; want one deletion", deleted, err)
	}
	if deleted, err := service.CleanupAllIssueTasks(t.Context()); err != nil || deleted != 1 {
		t.Fatalf("issue cleanup = %d, %v; want one deletion", deleted, err)
	}
	if client.mrStatusCalls != 1 {
		t.Fatalf("review status calls = %d, want 1 for the active task", client.mrStatusCalls)
	}
	if client.issueStateCalls != 1 {
		t.Fatalf("issue status calls = %d, want 1 for the active task", client.issueStateCalls)
	}
	if len(recorder.taskIDs) != 2 || recorder.taskIDs[0] != "review-active" || recorder.taskIDs[1] != "issue-active" {
		t.Fatalf("deleted task IDs = %v, want active tasks only", recorder.taskIDs)
	}
}
