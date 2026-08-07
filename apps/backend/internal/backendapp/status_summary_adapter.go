package backendapp

import (
	"context"
	"strconv"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// githubTaskStatusSummaryPRReader keeps task-service summary hydration
// independent from the GitHub package's storage model.
type githubTaskStatusSummaryPRReader struct {
	gh *github.Service
}

func (r *githubTaskStatusSummaryPRReader) ListTaskStatusSummaryPullRequests(
	ctx context.Context,
	taskIDs []string,
) (map[string][]statussummary.PullRequestInput, error) {
	result := make(map[string][]statussummary.PullRequestInput)
	if r == nil || r.gh == nil || len(taskIDs) == 0 {
		return result, nil
	}
	prsByTask, err := r.gh.ListTaskPRsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for taskID, prs := range prsByTask {
		for _, pr := range prs {
			if pr == nil {
				continue
			}
			key := pr.ID
			if key == "" {
				key = pr.RepositoryID + ":" + strconv.Itoa(pr.PRNumber)
			}
			requiredReviews := 0
			if pr.RequiredReviews != nil {
				requiredReviews = *pr.RequiredReviews
			}
			result[taskID] = append(result[taskID], statussummary.PullRequestInput{
				Key:                   key,
				State:                 pr.State,
				Number:                pr.PRNumber,
				URL:                   pr.PRURL,
				ReviewState:           pr.ReviewState,
				ChecksState:           pr.ChecksState,
				MergeableState:        pr.MergeableState,
				UnresolvedReviewCount: pr.UnresolvedReviewThreads,
				PendingReviewCount:    pr.PendingReviewCount,
				RequiredReviews:       requiredReviews,
				ChecksTotal:           pr.ChecksTotal,
				ChecksPassing:         pr.ChecksPassing,
			})
		}
	}
	return result, nil
}
