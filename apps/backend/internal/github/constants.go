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

// Check source identifiers.
const (
	checkSourceCheckRun      = "check_run"
	checkSourceStatusContext = "status_context"
)

// PR comment source identifiers.
const (
	commentTypeReview = "review"
	commentTypeIssue  = "issue"
)

// Reviewer types.
const (
	reviewerTypeUser = "user"
	reviewerTypeTeam = "team"
)

// GitHub user type identifiers.
const (
	githubUserTypeBot = "Bot"
)

// Commit status state values from /commits/:ref/status.
const (
	commitStatusSuccess = "success"
	commitStatusPending = "pending"
	commitStatusFailure = "failure"
	commitStatusError   = "error"
)

// Watch poll interval bounds in seconds (stored in the database).
const (
	defaultWatchPollIntervalSec = 300
	minWatchPollIntervalSec     = 60
)
