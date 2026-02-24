package github

// Normalized PR states (lowercase, used after conversion from GitHub API).
const (
	prStateMerged = "merged"
	prStateClosed = "closed"
)

// PR review states from the GitHub API.
const (
	reviewStateApproved         = "APPROVED"
	reviewStateChangesRequested = "CHANGES_REQUESTED"
	reviewStatePending          = "PENDING"
	reviewStateCommented        = "COMMENTED"
)

// Computed (aggregated) review states.
const (
	computedReviewStateApproved         = "approved"
	computedReviewStateChangesRequested = "changes_requested"
	computedReviewStatePending          = "pending"
)

// Check run status and conclusion values from the GitHub API.
const (
	checkStatusCompleted = "completed"
	checkStatusPending   = "pending"
	checkStatusSuccess   = "success"
	checkConclusionFail  = "failure"
)

// Watch poll interval bounds in seconds (stored in the database).
const (
	defaultWatchPollIntervalSec = 300
	minWatchPollIntervalSec     = 60
)
