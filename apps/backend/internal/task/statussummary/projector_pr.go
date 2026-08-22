package statussummary

import (
	"fmt"
	"sort"
	"strings"
)

func applyPullRequestInputs(state *projectionState, inputs []PullRequestInput) {
	for index, input := range inputs {
		key := strings.TrimSpace(input.Key)
		if key == "" {
			key = strings.TrimSpace(input.URL)
		}
		if key == "" && input.Number > 0 {
			key = fmt.Sprintf("#%d", input.Number)
		}
		if key == "" {
			key = fmt.Sprintf("index:%d", index)
		}
		state.prs[key] = pullRequestObservation{
			state:                 input.State,
			number:                maxInt(input.Number, 0),
			url:                   input.URL,
			reviewState:           input.ReviewState,
			checksState:           input.ChecksState,
			mergeableState:        input.MergeableState,
			mergeQueueState:       input.MergeQueueState,
			unresolvedReviewCount: maxInt(input.UnresolvedReviewCount, 0),
			pendingReviewCount:    maxInt(input.PendingReviewCount, 0),
			requiredReviews:       maxInt(input.RequiredReviews, 0),
			checksTotal:           maxInt(input.ChecksTotal, 0),
			checksPassing:         maxInt(input.ChecksPassing, 0),
		}
	}
}

func derivePullRequestSummary(state *projectionState) *PullRequestSummary {
	if !state.prObserved && state.prBaseline != nil {
		copy := *state.prBaseline
		return &copy
	}
	var summary PullRequestSummary
	var representative *pullRequestObservation
	keys := make([]string, 0, len(state.prs))
	for key := range state.prs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		observation := state.prs[key]
		summary.Count++
		if strings.EqualFold(observation.state, prStateOpen) {
			summary.OpenCount++
		}
		if pullRequestNeedsAttention(observation) {
			summary.Attention = true
		}
		if representative == nil || betterPRRepresentative(observation, *representative) {
			copy := observation
			representative = &copy
		}
	}
	if representative == nil {
		return nil
	}
	summary.State = representative.state
	summary.Number = representative.number
	summary.URL = truncateString(representative.url, maxPullRequestURLBytes)
	summary.AggregateState = aggregatePullRequestState(state.prs)
	return &summary
}

func pullRequestNeedsAttention(pr pullRequestObservation) bool {
	if strings.EqualFold(pr.reviewState, prStateChanges) ||
		strings.EqualFold(pr.checksState, prStateFailure) ||
		strings.EqualFold(pr.mergeableState, prStateBlocked) ||
		strings.EqualFold(pr.mergeableState, prStateDirty) ||
		pr.unresolvedReviewCount > 0 {
		return true
	}
	if pr.requiredReviews > 0 && pr.pendingReviewCount > 0 {
		return true
	}
	return strings.EqualFold(pr.checksState, prStatePending) ||
		(pr.checksTotal > 0 && pr.checksPassing < pr.checksTotal)
}

func betterPRRepresentative(candidate, current pullRequestObservation) bool {
	candidateOpen := strings.EqualFold(candidate.state, prStateOpen)
	currentOpen := strings.EqualFold(current.state, prStateOpen)
	if candidateOpen != currentOpen {
		return candidateOpen
	}
	candidateAttention := pullRequestNeedsAttention(candidate)
	currentAttention := pullRequestNeedsAttention(current)
	if candidateAttention != currentAttention {
		return candidateAttention
	}
	if candidate.number != current.number {
		return candidate.number < current.number
	}
	return candidate.url < current.url
}

func aggregatePullRequestState(prs map[string]pullRequestObservation) string {
	best := prStateNeutral
	bestRank := 0
	for _, pr := range prs {
		state := pullRequestAggregateState(pr)
		rank := pullRequestStateRank(state)
		if rank > bestRank {
			best = state
			bestRank = rank
		}
	}
	return best
}

func pullRequestAggregateState(pr pullRequestObservation) string {
	state := strings.ToLower(strings.TrimSpace(pr.state))
	mergeable := strings.ToLower(strings.TrimSpace(pr.mergeableState))
	checks := strings.ToLower(strings.TrimSpace(pr.checksState))
	review := strings.ToLower(strings.TrimSpace(pr.reviewState))
	if state == prStateMerged || state == prStateClosed {
		return pullRequestLifecycleState(state, mergeable)
	}
	if strings.TrimSpace(pr.mergeQueueState) != "" {
		return prStateQueued
	}
	if lifecycle := pullRequestLifecycleState(state, mergeable); lifecycle != "" {
		return lifecycle
	}
	if mergeable == prStateBlocked || mergeable == prStateDirty {
		return prStateBlocked
	}
	if pullRequestHasFailure(pr, review, checks) {
		return prStateFailure
	}
	if pullRequestHasPendingChecks(pr, checks) {
		return prStatePending
	}
	if pullRequestAwaitsReview(pr, review) {
		return prStateAwaiting
	}
	if review == prStateApproved && (checks == "" || checks == prStateSuccess) {
		return prStateReady
	}
	if checks == prStateSuccess {
		return prStatePassing
	}
	if state == prStateOpen {
		return prStateReady
	}
	return prStateNeutral
}

func pullRequestLifecycleState(state, mergeable string) string {
	switch {
	case state == prStateMerged:
		return prStateMerged
	case state == prStateClosed:
		return prStateClosed
	case state == prStateDraft || mergeable == prStateDraft:
		return prStateDraft
	default:
		return ""
	}
}

func pullRequestHasFailure(pr pullRequestObservation, review, checks string) bool {
	return review == prStateChanges || checks == prStateFailure || pr.unresolvedReviewCount > 0
}

func pullRequestHasPendingChecks(pr pullRequestObservation, checks string) bool {
	return checks == prStatePending ||
		(pr.checksTotal > 0 && pr.checksPassing < pr.checksTotal && checks != prStateSuccess)
}

func pullRequestAwaitsReview(pr pullRequestObservation, review string) bool {
	return pr.pendingReviewCount > 0 || (pr.requiredReviews > 0 && review == prStatePending)
}

func pullRequestStateRank(state string) int {
	switch state {
	case prStateFailure:
		return 100
	case prStateBlocked:
		return 90
	case prStatePending:
		return 80
	case prStateAwaiting:
		return 70
	case prStateReady:
		return 60
	case prStateQueued:
		return 55
	case prStatePassing:
		return 50
	case prStateDraft:
		return 40
	case prStateMerged:
		return 20
	case prStateClosed:
		return 10
	default:
		return 0
	}
}
