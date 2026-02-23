// Package github provides GitHub integration for Kandev, including PR monitoring,
// review queue management, and CI/check status tracking.
package github

import "time"

// PR represents a GitHub Pull Request.
type PR struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	HTMLURL     string     `json:"html_url"`
	State       string     `json:"state"` // open, closed, merged
	HeadBranch  string     `json:"head_branch"`
	HeadSHA     string     `json:"head_sha"`
	BaseBranch  string     `json:"base_branch"`
	AuthorLogin string     `json:"author_login"`
	RepoOwner   string     `json:"repo_owner"`
	RepoName    string     `json:"repo_name"`
	Draft       bool       `json:"draft"`
	Mergeable   bool       `json:"mergeable"`
	Additions   int        `json:"additions"`
	Deletions   int        `json:"deletions"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	MergedAt    *time.Time `json:"merged_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

// PRReview represents a review on a PR.
type PRReview struct {
	ID           int64     `json:"id"`
	Author       string    `json:"author"`
	AuthorAvatar string    `json:"author_avatar"`
	State        string    `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, PENDING, DISMISSED
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
}

// PRComment represents a review comment on specific code.
type PRComment struct {
	ID           int64     `json:"id"`
	Author       string    `json:"author"`
	AuthorAvatar string    `json:"author_avatar"`
	Body         string    `json:"body"`
	Path         string    `json:"path"`
	Line         int       `json:"line"`
	Side         string    `json:"side"` // LEFT, RIGHT
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	InReplyTo    *int64    `json:"in_reply_to,omitempty"`
}

// CheckRun represents a CI check result.
type CheckRun struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`     // queued, in_progress, completed
	Conclusion  string     `json:"conclusion"` // success, failure, neutral, cancelled, timed_out, action_required, skipped
	HTMLURL     string     `json:"html_url"`
	Output      string     `json:"output"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// PRFeedback aggregates all feedback for a PR (fetched live from GitHub).
type PRFeedback struct {
	PR        *PR         `json:"pr"`
	Reviews   []PRReview  `json:"reviews"`
	Comments  []PRComment `json:"comments"`
	Checks    []CheckRun  `json:"checks"`
	HasIssues bool        `json:"has_issues"`
}

// PRWatch tracks active PR monitoring (session → PR).
type PRWatch struct {
	ID              string     `json:"id" db:"id"`
	SessionID       string     `json:"session_id" db:"session_id"`
	TaskID          string     `json:"task_id" db:"task_id"`
	Owner           string     `json:"owner" db:"owner"`
	Repo            string     `json:"repo" db:"repo"`
	PRNumber        int        `json:"pr_number" db:"pr_number"`
	Branch          string     `json:"branch" db:"branch"`
	LastCheckedAt   *time.Time `json:"last_checked_at,omitempty" db:"last_checked_at"`
	LastCommentAt   *time.Time `json:"last_comment_at,omitempty" db:"last_comment_at"`
	LastCheckStatus string     `json:"last_check_status" db:"last_check_status"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskPR associates a PR with a task.
type TaskPR struct {
	ID                 string     `json:"id" db:"id"`
	TaskID             string     `json:"task_id" db:"task_id"`
	Owner              string     `json:"owner" db:"owner"`
	Repo               string     `json:"repo" db:"repo"`
	PRNumber           int        `json:"pr_number" db:"pr_number"`
	PRURL              string     `json:"pr_url" db:"pr_url"`
	PRTitle            string     `json:"pr_title" db:"pr_title"`
	HeadBranch         string     `json:"head_branch" db:"head_branch"`
	BaseBranch         string     `json:"base_branch" db:"base_branch"`
	AuthorLogin        string     `json:"author_login" db:"author_login"`
	State              string     `json:"state" db:"state"`               // open, closed, merged
	ReviewState        string     `json:"review_state" db:"review_state"` // approved, changes_requested, pending, ""
	ChecksState        string     `json:"checks_state" db:"checks_state"` // success, failure, pending, ""
	ReviewCount        int        `json:"review_count" db:"review_count"`
	PendingReviewCount int        `json:"pending_review_count" db:"pending_review_count"`
	CommentCount       int        `json:"comment_count" db:"comment_count"`
	Additions          int        `json:"additions" db:"additions"`
	Deletions          int        `json:"deletions" db:"deletions"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	MergedAt           *time.Time `json:"merged_at,omitempty" db:"merged_at"`
	ClosedAt           *time.Time `json:"closed_at,omitempty" db:"closed_at"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty" db:"last_synced_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
}

// RepoFilter identifies a GitHub repository for review watch filtering.
type RepoFilter struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// ReviewScope controls which GitHub search qualifier is used for review-requested PRs.
const (
	// ReviewScopeUser matches only PRs where the user is explicitly requested
	// (user-review-requested:@me).
	ReviewScopeUser = "user"
	// ReviewScopeUserAndTeams matches PRs requested from the user or any of their teams
	// (review-requested:@me). This is the default for backwards compatibility.
	ReviewScopeUserAndTeams = "user_and_teams"
)

// ReviewWatch configures periodic polling for PRs needing the user's review.
// Repos holds the list of repositories to monitor. An empty list means all repos.
type ReviewWatch struct {
	ID                  string       `json:"id" db:"id"`
	WorkspaceID         string       `json:"workspace_id" db:"workspace_id"`
	WorkflowID          string       `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID      string       `json:"workflow_step_id" db:"workflow_step_id"`
	Repos               []RepoFilter `json:"repos" db:"-"`
	ReposJSON           string       `json:"-" db:"repos"`
	AgentProfileID      string       `json:"agent_profile_id" db:"agent_profile_id"`
	ExecutorProfileID   string       `json:"executor_profile_id" db:"executor_profile_id"`
	Prompt              string       `json:"prompt" db:"prompt"`
	ReviewScope         string       `json:"review_scope" db:"review_scope"`
	CustomQuery         string       `json:"custom_query" db:"custom_query"`
	Enabled             bool         `json:"enabled" db:"enabled"`
	PollIntervalSeconds int          `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	LastPolledAt        *time.Time   `json:"last_polled_at,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at" db:"updated_at"`
}

// ReviewPRTask records which PRs have already had tasks created (deduplication).
type ReviewPRTask struct {
	ID            string    `json:"id" db:"id"`
	ReviewWatchID string    `json:"review_watch_id" db:"review_watch_id"`
	RepoOwner     string    `json:"repo_owner" db:"repo_owner"`
	RepoName      string    `json:"repo_name" db:"repo_name"`
	PRNumber      int       `json:"pr_number" db:"pr_number"`
	PRURL         string    `json:"pr_url" db:"pr_url"`
	TaskID        string    `json:"task_id" db:"task_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// GitHubOrg represents a GitHub organization the authenticated user belongs to.
type GitHubOrg struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubRepo represents a GitHub repository (lightweight, for autocomplete).
type GitHubRepo struct {
	FullName string `json:"full_name"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	Private  bool   `json:"private"`
}

// GitHubStatus represents GitHub connection status.
type GitHubStatus struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
	AuthMethod    string `json:"auth_method"` // "gh_cli", "pat", "none"
}

// CreateReviewWatchRequest is the request body for creating a review watch.
type CreateReviewWatchRequest struct {
	WorkspaceID         string       `json:"workspace_id"`
	WorkflowID          string       `json:"workflow_id"`
	WorkflowStepID      string       `json:"workflow_step_id"`
	Repos               []RepoFilter `json:"repos"`
	AgentProfileID      string       `json:"agent_profile_id"`
	ExecutorProfileID   string       `json:"executor_profile_id"`
	Prompt              string       `json:"prompt"`
	ReviewScope         string       `json:"review_scope"`
	CustomQuery         string       `json:"custom_query"`
	PollIntervalSeconds int          `json:"poll_interval_seconds"`
}

// UpdateReviewWatchRequest is the request body for updating a review watch.
type UpdateReviewWatchRequest struct {
	WorkflowID          *string       `json:"workflow_id,omitempty"`
	WorkflowStepID      *string       `json:"workflow_step_id,omitempty"`
	Repos               *[]RepoFilter `json:"repos,omitempty"`
	AgentProfileID      *string       `json:"agent_profile_id,omitempty"`
	ExecutorProfileID   *string       `json:"executor_profile_id,omitempty"`
	Prompt              *string       `json:"prompt,omitempty"`
	ReviewScope         *string       `json:"review_scope,omitempty"`
	CustomQuery         *string       `json:"custom_query,omitempty"`
	Enabled             *bool         `json:"enabled,omitempty"`
	PollIntervalSeconds *int          `json:"poll_interval_seconds,omitempty"`
}

// PRFeedbackEvent is published to the event bus when a PR has new feedback.
type PRFeedbackEvent struct {
	SessionID      string `json:"session_id"`
	TaskID         string `json:"task_id"`
	PRNumber       int    `json:"pr_number"`
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	NewComments    int    `json:"new_comments"`
	ChecksChanged  bool   `json:"checks_changed"`
	NewCheckStatus string `json:"new_check_status"`
	NewReviewState string `json:"new_review_state"`
}

// NewReviewPREvent is published when a new PR needing review is found.
type NewReviewPREvent struct {
	ReviewWatchID     string `json:"review_watch_id"`
	WorkspaceID       string `json:"workspace_id"`
	WorkflowID        string `json:"workflow_id"`
	WorkflowStepID    string `json:"workflow_step_id"`
	AgentProfileID    string `json:"agent_profile_id"`
	ExecutorProfileID string `json:"executor_profile_id"`
	Prompt            string `json:"prompt"`
	PR                *PR    `json:"pr"`
}

// PRStatsRequest defines filters for PR stats queries.
type PRStatsRequest struct {
	WorkspaceID string     `json:"workspace_id"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
}

// PRStats holds aggregated PR analytics.
type PRStats struct {
	TotalPRsCreated     int          `json:"total_prs_created"`
	TotalPRsReviewed    int          `json:"total_prs_reviewed"`
	TotalComments       int          `json:"total_comments"`
	CIPassRate          float64      `json:"ci_pass_rate"`
	ApprovalRate        float64      `json:"approval_rate"`
	AvgTimeToMergeHours float64      `json:"avg_time_to_merge_hours"`
	PRsByDay            []DailyCount `json:"prs_by_day"`
}

// DailyCount holds a date and count for chart data.
type DailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
