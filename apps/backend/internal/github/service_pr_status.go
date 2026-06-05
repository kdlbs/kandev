package github

import "time"

func findLatestCommentTime(comments []PRComment) *time.Time {
	var latest *time.Time
	for _, c := range comments {
		t := c.UpdatedAt
		if latest == nil || t.After(*latest) {
			latest = &t
		}
	}
	return latest
}

// computeOverallCheckStatus reduces per-check runs to a single PR-level status.
// Mirrors GitHub's own UI: skipped/neutral conclusions are ignored; any failing
// terminal state (failure, timed_out, cancelled, action_required) makes the PR
// failed; non-completed checks keep the PR pending.
func computeOverallCheckStatus(checks []CheckRun) string {
	if len(checks) == 0 {
		return ""
	}
	hasPending := false
	hasPassing := false
	for _, c := range checks {
		if c.Status != checkStatusCompleted {
			hasPending = true
			continue
		}
		switch c.Conclusion {
		case checkConclusionFail, checkConclusionTimedOut,
			checkConclusionCancelled, checkConclusionActionRequired:
			return checkConclusionFail
		case checkConclusionSkipped, checkConclusionNeutral:
			// ignore — GitHub's UI does
		default:
			// Treat success and any future unknown terminal conclusion as passing.
			// Being permissive preserves the success signal if GitHub introduces
			// a new conclusion we haven't mapped yet.
			hasPassing = true
		}
	}
	if hasPending {
		return checkStatusPending
	}
	if hasPassing {
		return checkStatusSuccess
	}
	return ""
}

func computeOverallReviewState(reviews []PRReview) string {
	if len(reviews) == 0 {
		return ""
	}
	latest := latestReviewByAuthor(reviews)
	changesReq := false
	allApproved := true
	for _, r := range latest {
		if r.State == reviewStateChangesRequested {
			changesReq = true
		}
		if r.State != reviewStateApproved {
			allApproved = false
		}
	}
	if changesReq {
		return computedReviewStateChangesRequested
	}
	if allApproved {
		return computedReviewStateApproved
	}
	return computedReviewStatePending
}

func countPendingReviews(reviews []PRReview) int {
	latest := latestReviewByAuthor(reviews)
	count := 0
	for _, r := range latest {
		if r.State == reviewStatePending || r.State == reviewStateCommented {
			count++
		}
	}
	return count
}

func countPendingRequestedReviewers(pr *PR) int {
	if pr == nil {
		return 0
	}
	return len(pr.RequestedReviewers)
}

func deriveReviewSyncState(pr *PR, reviews []PRReview) (string, int) {
	pendingReviewCount := countPendingRequestedReviewers(pr)
	if pendingReviewCount == 0 {
		pendingReviewCount = countPendingReviews(reviews)
	}
	reviewState := computeOverallReviewState(reviews)
	if reviewState == "" && pendingReviewCount > 0 {
		reviewState = computedReviewStatePending
	}
	return reviewState, pendingReviewCount
}
