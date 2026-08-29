package backendapp

import (
	"context"
	"fmt"

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
	automationByTask, err := r.gh.ListTaskPRAutomationOptionsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for taskID, prs := range prsByTask {
		optionsByKey := make(map[string]*github.TaskPRAutomationOptions, len(automationByTask[taskID]))
		for _, options := range automationByTask[taskID] {
			if options == nil {
				continue
			}
			optionsByKey[fmt.Sprintf("%s#%d", options.RepositoryID, options.PRNumber)] = options
		}
		for _, pr := range prs {
			if pr == nil {
				continue
			}
			key := taskStatusSummaryPRKey(pr)
			if key == "" {
				continue
			}
			requiredReviews := 0
			if pr.RequiredReviews != nil {
				requiredReviews = *pr.RequiredReviews
			}
			options := optionsByKey[key]
			result[taskID] = append(result[taskID], statussummary.PullRequestInput{
				Key:                   key,
				State:                 pr.State,
				Number:                pr.PRNumber,
				URL:                   pr.PRURL,
				ReviewState:           pr.ReviewState,
				ChecksState:           pr.ChecksState,
				MergeableState:        pr.MergeableState,
				MergeQueueState:       pr.MergeQueueState,
				UnresolvedReviewCount: pr.UnresolvedReviewThreads,
				PendingReviewCount:    pr.PendingReviewCount,
				RequiredReviews:       requiredReviews,
				ChecksTotal:           pr.ChecksTotal,
				ChecksPassing:         pr.ChecksPassing,
				AutoFixEnabled:        options != nil && options.AutoFixEnabled,
				AutoMergeEnabled:      options != nil && options.AutoMergeEnabled,
			})
		}
	}
	return result, nil
}

// taskStatusSummaryPRKey mirrors the live projector's event identity so an
// update replaces its rehydrated observation instead of creating a duplicate.
func taskStatusSummaryPRKey(pr *github.TaskPR) string {
	if pr == nil {
		return ""
	}
	if pr.RepositoryID != "" && pr.PRNumber > 0 {
		return fmt.Sprintf("%s#%d", pr.RepositoryID, pr.PRNumber)
	}
	if pr.PRURL != "" {
		return pr.PRURL
	}
	if pr.RepositoryID != "" {
		return pr.RepositoryID
	}
	if pr.PRNumber > 0 {
		return fmt.Sprintf("#%d", pr.PRNumber)
	}
	return ""
}
