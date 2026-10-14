package github

import (
	"context"
	"testing"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.6
func TestPRFeedbackPreservesStatusFreshness(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-time.Hour)
	fresh := now.Add(-time.Second)
	tests := []struct {
		name       string
		syncedAt   *time.Time
		headSHA    string
		baseBranch string
		wantSynced *time.Time
	}{
		{"stale status", &stale, feedbackSyncHead, "main", &stale},
		{"fresh status", &fresh, feedbackSyncHead, "main", &fresh},
		{"missing status", nil, feedbackSyncHead, "main", nil},
		{"changed head", &fresh, "previous-head", "main", nil},
		{"changed base", &fresh, feedbackSyncHead, "previous-base", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, svc, client, store := setupPollerTest(t)
			ctx := context.Background()
			seedTask(t, store, "task-1", false)
			seedFeedbackTaskPR(t, store, "task-1", 2408, func(pr *TaskPR) {
				pr.LastSyncedAt = tt.syncedAt
				pr.HeadSHA = tt.headSHA
				pr.BaseBranch = tt.baseBranch
				pr.ChecksState = checkStatusPending
			})
			client.AddRepos("test-user", []GitHubRepo{{
				Owner: feedbackSyncOwner, Name: feedbackSyncRepo, FullName: feedbackSyncOwner + "/" + feedbackSyncRepo,
			}})
			client.AddPR(&PR{
				Number: 2408, State: prStateOpen, Title: "Ready with an unresolved comment",
				RepoOwner: feedbackSyncOwner, RepoName: feedbackSyncRepo,
				HeadSHA: feedbackSyncHead, BaseBranch: "main", MergeableState: "clean",
			})
			client.AddReviews(feedbackSyncOwner, feedbackSyncRepo, 2408, []PRReview{
				{ID: 1, Author: "reviewer", State: reviewStateApproved},
			})
			client.AddCheckRuns(feedbackSyncOwner, feedbackSyncRepo, feedbackSyncHead, []CheckRun{
				{Name: "build", Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
			})
			client.AddComments(feedbackSyncOwner, feedbackSyncRepo, 2408, []PRComment{
				{ID: 99, Author: "reviewer", Body: "Fix this unresolved comment", Path: "app.go", Line: 1},
			})
			updated := subscribeTaskPRUpdated(t, svc)
			if _, err := svc.GetPRFeedbackForAutomation(ctx, testWorkspaceID, feedbackSyncOwner, feedbackSyncRepo, 2408); err != nil {
				t.Fatal(err)
			}
			event := awaitTaskPRUpdated(t, updated)
			got, err := store.GetTaskPRByRepoAndNumber(ctx, "task-1", "repo-1", 2408)
			if err != nil || got == nil {
				t.Fatalf("get task PR: %v (row %+v)", err, got)
			}
			if !timeEqual(got.LastSyncedAt, tt.wantSynced) || !timeEqual(event.LastSyncedAt, tt.wantSynced) {
				t.Fatalf("stored/event sync time = %v/%v, want %v", got.LastSyncedAt, event.LastSyncedAt, tt.wantSynced)
			}
			if got.ChecksState != checkStatusSuccess || got.ReviewState != computedReviewStateApproved || got.UnresolvedReviewThreads != 0 {
				t.Fatalf("feedback status = %+v, want successful checks, approved review, and preserved thread count", got)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.7
func TestAutomationFeedbackInvalidationPreservesSiblingCache(t *testing.T) {
	_, svc, client, _ := setupPollerTest(t)
	ctx := context.Background()
	for _, number := range []int{2408, 2409} {
		addMergedFeedbackPR(client, number, time.Now().UTC())
		if _, err := svc.GetPRFeedbackForAutomation(ctx, testWorkspaceID, feedbackSyncOwner, feedbackSyncRepo, number); err != nil {
			t.Fatal(err)
		}
		client.AddComments(feedbackSyncOwner, feedbackSyncRepo, number, []PRComment{
			{ID: int64(number), Author: "reviewer", Body: "New actionable comment"},
		})
	}
	if err := svc.InvalidatePRFeedbackForAutomation(ctx, testWorkspaceID, feedbackSyncOwner, feedbackSyncRepo, 2408); err != nil {
		t.Fatal(err)
	}
	for _, number := range []int{2408, 2409} {
		feedback, err := svc.GetPRFeedbackForAutomation(ctx, testWorkspaceID, feedbackSyncOwner, feedbackSyncRepo, number)
		if err != nil {
			t.Fatal(err)
		}
		if number == 2408 {
			if len(feedback.Comments) != 1 || feedback.Comments[0].ID != 2408 {
				t.Fatalf("target feedback = %+v, want newly fetched comment", feedback.Comments)
			}
		} else if len(feedback.Comments) != 0 {
			t.Fatalf("sibling cache was invalidated: %+v", feedback.Comments)
		}
	}
}
