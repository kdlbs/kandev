package github

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7
func TestPRWatchCommentOnlyUpdatesPublishFeedbackOnce(t *testing.T) {
	for _, path := range []string{"REST", "GraphQL"} {
		t.Run(path, func(t *testing.T) {
			poller, service, _, store := setupPollerTest(t)
			ctx := context.Background()
			seedTask(t, store, "task-comment-only", false)
			watch, err := service.CreatePRWatchForWorkspace(
				ctx, testWorkspaceID, "session-comment-only", "task-comment-only", "repo-1", "acme", "widget", 42, "feature",
			)
			require.NoError(t, err)
			watermark := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
			require.NoError(t, store.UpdatePRWatchTimestamps(ctx, watch.ID, watermark, &watermark, "success", "approved"))

			var feedbackCount atomic.Int32
			subscription, err := service.eventBus.Subscribe(events.GitHubPRFeedback, func(context.Context, *bus.Event) error {
				feedbackCount.Add(1)
				return nil
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = subscription.Unsubscribe() })

			createdAt := watermark.Add(-time.Hour)
			requiredReviews := 1
			status := &PRStatus{
				PR: &PR{
					Number: 42, Title: "Stable PR", State: "open", RepoOwner: "acme", RepoName: "widget",
					HTMLURL: "https://github.com/acme/widget/pull/42", HeadBranch: "feature", BaseBranch: "main",
					HeadSHA: "head-sha", AuthorLogin: "reviewer", CreatedAt: createdAt,
					UpdatedAt: watermark.Add(time.Minute),
				},
				ChecksState: "success", ReviewState: "approved", RequiredReviews: &requiredReviews,
			}
			apply := func(observation *PRStatus) {
				t.Helper()
				switch path {
				case "REST":
					require.NoError(t, poller.applyPRStatusForWatch(ctx, watch, observation))
				case "GraphQL":
					key := prStatusCacheKey(watch.Owner, watch.Repo, watch.PRNumber)
					result := service.applyBatchedNumberedWatch(
						ctx, "comment-only-test", watch, map[string]*PRStatus{key: observation},
						time.Now().UTC(), true,
					)
					require.True(t, result.Applied)
				}
			}

			apply(status)
			feedbackCount.Store(0)
			commentOnly := *status
			prCopy := *status.PR
			commentOnly.PR = &prCopy
			commentOnly.PR.UpdatedAt = status.PR.UpdatedAt.Add(time.Minute)

			apply(&commentOnly)
			require.EqualValues(t, 1, feedbackCount.Load(), "a new PR updated_at must publish comment feedback")
			apply(&commentOnly)
			require.EqualValues(t, 1, feedbackCount.Load(), "the consumed watermark must suppress a duplicate event")
		})
	}
}
