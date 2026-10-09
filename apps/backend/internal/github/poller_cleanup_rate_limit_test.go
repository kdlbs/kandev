package github

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

func TestCleanupPollCycleSkipsRemainingCleanupAfterRateLimit(t *testing.T) {
	t.Run("review per-watch and orphan cleanup", func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{feedback: map[int]cleanupPRResponse{
			1: {err: rateLimitAPIError(http.StatusTooManyRequests, "rate limit exceeded")},
		}}
		configureCleanupRateLimitClient(t, svc, client, nil)
		ctx := context.Background()

		first := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
		second := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
		orphan := &ReviewWatch{WorkspaceID: "ws-1", Enabled: false, CleanupPolicy: CleanupPolicyAlways}
		for _, watch := range []*ReviewWatch{first, second, orphan} {
			if err := store.CreateReviewWatch(ctx, watch); err != nil {
				t.Fatalf("CreateReviewWatch: %v", err)
			}
		}
		createReviewCleanupRows(t, store, first, "review-first")
		createReviewCleanupRows(t, store, second, "review-second")
		createReviewCleanupRows(t, store, orphan, "review-orphan")
		recorder := &recordingTaskDeleter{}
		svc.SetTaskDeleter(recorder)

		poller.checkReviewWatches(ctx)
		if !reflect.DeepEqual(client.prCalls, []int{1}) {
			t.Fatalf("GetPR calls = %v, want only first watch", client.prCalls)
		}
		if len(recorder.calls) != 0 {
			t.Fatalf("DeleteTask calls = %v, want none after rate-limit stop", recorder.calls)
		}
	})

	t.Run("issue per-watch and orphan cleanup", func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		client := &cleanupRateLimitClient{issues: map[int]cleanupIssueResponse{
			1: {err: rateLimitAPIError(http.StatusForbidden, "abuse detection")},
		}}
		configureCleanupRateLimitClient(t, svc, client, nil)
		ctx := context.Background()

		first := &IssueWatch{WorkspaceID: "ws-1", Enabled: true}
		second := &IssueWatch{WorkspaceID: "ws-1", Enabled: true}
		orphan := &IssueWatch{WorkspaceID: "ws-1", Enabled: false}
		for _, watch := range []*IssueWatch{first, second, orphan} {
			if err := store.CreateIssueWatch(ctx, watch); err != nil {
				t.Fatalf("CreateIssueWatch: %v", err)
			}
		}
		createIssueCleanupRows(t, store, first, "issue-first")
		createIssueCleanupRows(t, store, second, "issue-second")
		createIssueCleanupRows(t, store, orphan, "issue-orphan")
		recorder := &recordingTaskDeleter{}
		svc.SetTaskDeleter(recorder)

		poller.checkIssueWatches(ctx)
		if !reflect.DeepEqual(client.issueCalls, []int{1}) {
			t.Fatalf("issue calls = %v, want only first watch", client.issueCalls)
		}
		if len(recorder.calls) != 0 {
			t.Fatalf("DeleteTask calls = %v, want none after rate-limit stop", recorder.calls)
		}
	})
}
