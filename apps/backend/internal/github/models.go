// Package github provides GitHub integration for Kandev, including PR monitoring,
// review queue management, and CI/check status tracking.
package github

import (
	"encoding/json"
	"time"
)

type AppRegistrationOwnerType string

const (
	AppRegistrationOwnerUser         AppRegistrationOwnerType = "User"
	AppRegistrationOwnerOrganization AppRegistrationOwnerType = "Organization"
)

type DeploymentAppOwnerType = AppRegistrationOwnerType

const (
	DeploymentAppOwnerUser         = AppRegistrationOwnerUser
	DeploymentAppOwnerOrganization = AppRegistrationOwnerOrganization
)

type AppRegistrationSource string

const (
	AppRegistrationSourceManaged  AppRegistrationSource = "managed"
	AppRegistrationSourceImported AppRegistrationSource = "imported"
)

type AppRegistrationVisibility string

const (
	AppRegistrationVisibilityPrivate AppRegistrationVisibility = "private"
	AppRegistrationVisibilityPublic  AppRegistrationVisibility = "public"
)

type AppRegistrationStatus string

const (
	AppRegistrationStatusActive  AppRegistrationStatus = "active"
	AppRegistrationStatusInvalid AppRegistrationStatus = "invalid"
)

type DeploymentAppWebhookStatus string

const (
	DeploymentAppWebhookUnverified DeploymentAppWebhookStatus = "unverified"
	DeploymentAppWebhookVerified   DeploymentAppWebhookStatus = "verified"
	DeploymentAppWebhookFailing    DeploymentAppWebhookStatus = "failing"
)

// AppRegistration is non-secret metadata for one GitHub App in the deployment catalog.
type AppRegistration struct {
	ID                    string                     `json:"id" db:"id"`
	Source                AppRegistrationSource      `json:"source" db:"source"`
	DisplayName           string                     `json:"display_name" db:"display_name"`
	GitHubHost            string                     `json:"github_host" db:"github_host"`
	AppID                 int64                      `json:"app_id" db:"app_id"`
	ClientID              string                     `json:"client_id" db:"client_id"`
	Slug                  string                     `json:"slug" db:"slug"`
	OwnerLogin            string                     `json:"owner_login" db:"owner_login"`
	OwnerType             AppRegistrationOwnerType   `json:"owner_type" db:"owner_type"`
	Visibility            AppRegistrationVisibility  `json:"visibility" db:"visibility"`
	PublicBaseURL         string                     `json:"public_base_url" db:"public_base_url"`
	CreatedForWorkspaceID string                     `json:"created_for_workspace_id,omitempty" db:"created_for_workspace_id"`
	CredentialGeneration  int64                      `json:"credential_generation" db:"credential_generation"`
	CredentialSecretID    string                     `json:"-" db:"credential_secret_id"`
	Status                AppRegistrationStatus      `json:"status" db:"status"`
	WebhookStatus         DeploymentAppWebhookStatus `json:"webhook_status" db:"webhook_status"`
	LastWebhookAt         *time.Time                 `json:"last_webhook_at,omitempty" db:"last_webhook_at"`
	LastError             string                     `json:"last_error,omitempty" db:"last_error"`
	CreatedAt             time.Time                  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time                  `json:"updated_at" db:"updated_at"`
}

type DeploymentAppRegistration = AppRegistration

// DeploymentAppRegistrationFlow binds one manifest callback to its initiating operator.
type DeploymentAppRegistrationFlow struct {
	StateHash        string                    `json:"-" db:"state_hash"`
	RegistrationID   string                    `json:"registration_id" db:"registration_id"`
	WorkspaceID      string                    `json:"workspace_id" db:"workspace_id"`
	UserID           string                    `json:"-" db:"user_id"`
	OperatorUserID   string                    `json:"-" db:"operator_user_id"`
	OwnerType        DeploymentAppOwnerType    `json:"owner_type" db:"owner_type"`
	OwnerLogin       string                    `json:"owner_login" db:"owner_login"`
	DisplayName      string                    `json:"display_name" db:"display_name"`
	Visibility       AppRegistrationVisibility `json:"visibility" db:"visibility"`
	PublicBaseURL    string                    `json:"public_base_url" db:"public_base_url"`
	ManifestRevision int                       `json:"manifest_revision" db:"manifest_revision"`
	ExpiresAt        time.Time                 `json:"expires_at" db:"expires_at"`
	ConsumedAt       *time.Time                `json:"-" db:"consumed_at"`
	CreatedAt        time.Time                 `json:"created_at" db:"created_at"`
}

// AppRegistrationImportPreparation reserves one server-generated registration
// identity for a short-lived, single-use existing-App import.
type AppRegistrationImportPreparation struct {
	RegistrationID string     `json:"registration_id" db:"registration_id"`
	WorkspaceID    string     `json:"workspace_id" db:"workspace_id"`
	UserID         string     `json:"-" db:"user_id"`
	PublicBaseURL  string     `json:"public_base_url" db:"public_base_url"`
	ExpiresAt      time.Time  `json:"expires_at" db:"expires_at"`
	ConsumedAt     *time.Time `json:"-" db:"consumed_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}

// ConnectionSource identifies the credential source selected for workspace
// automation. Legacy shared auth is migration-only and cannot be selected for
// a new workspace.
type ConnectionSource string

const (
	ConnectionSourceLegacyShared          ConnectionSource = "legacy_shared"
	ConnectionSourcePAT                   ConnectionSource = "pat"
	ConnectionSourceGHCLI                 ConnectionSource = "gh_cli"
	ConnectionSourceGitHubAppInstallation ConnectionSource = "github_app_installation"
	ConnectionSourceGitHubAppUser         ConnectionSource = "github_app_user"
)

// ConnectionStatus is the persisted health/lifecycle state for a GitHub
// automation or personal connection.
type ConnectionStatus string

const (
	ConnectionStatusActive    ConnectionStatus = "active"
	ConnectionStatusInvalid   ConnectionStatus = "invalid"
	ConnectionStatusSuspended ConnectionStatus = "suspended"
	ConnectionStatusRevoked   ConnectionStatus = "revoked"
)

// WorkspaceConnection is the single automation identity selected by a
// workspace. Long-lived secret material is stored separately under a
// deterministic workspace-scoped secret key.
type WorkspaceConnection struct {
	WorkspaceID              string           `json:"workspace_id" db:"workspace_id"`
	Source                   ConnectionSource `json:"source" db:"source"`
	GitHubHost               string           `json:"github_host" db:"github_host"`
	Login                    string           `json:"login,omitempty" db:"login"`
	InstallationID           *int64           `json:"installation_id,omitempty" db:"installation_id"`
	InstallationAccountLogin string           `json:"installation_account_login,omitempty" db:"installation_account_login"`
	InstallationAccountType  string           `json:"installation_account_type,omitempty" db:"installation_account_type"`
	AppRegistrationID        string           `json:"app_registration_id,omitempty" db:"app_registration_id"`
	Status                   ConnectionStatus `json:"status" db:"status"`
	CredentialGeneration     int64            `json:"credential_generation" db:"credential_generation"`
	LastError                string           `json:"last_error,omitempty" db:"last_error"`
	CreatedAt                time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time        `json:"updated_at" db:"updated_at"`
}

// UserConnection is an optional personal GitHub identity for one Kandev user
// in one workspace. Its OAuth access and refresh tokens live in the encrypted
// secret store and are never placed in this row.
type UserConnection struct {
	WorkspaceID          string           `json:"workspace_id" db:"workspace_id"`
	UserID               string           `json:"user_id" db:"user_id"`
	AppRegistrationID    string           `json:"app_registration_id" db:"app_registration_id"`
	GitHubUserID         int64            `json:"github_user_id" db:"github_user_id"`
	Login                string           `json:"login" db:"login"`
	Status               ConnectionStatus `json:"status" db:"status"`
	AccessExpiresAt      time.Time        `json:"access_expires_at" db:"access_expires_at"`
	RefreshExpiresAt     *time.Time       `json:"refresh_expires_at,omitempty" db:"refresh_expires_at"`
	CredentialGeneration int64            `json:"credential_generation" db:"credential_generation"`
	LastError            string           `json:"last_error,omitempty" db:"last_error"`
	CreatedAt            time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at" db:"updated_at"`
}

type AuthFlowKind string

const (
	AuthFlowKindAppInstallation AuthFlowKind = "app_installation"
	AuthFlowKindPersonal        AuthFlowKind = "personal"
)

// AuthFlow stores one short-lived, single-use GitHub authorization attempt.
// StateHash stores a digest rather than the bearer state returned to a browser.
type AuthFlow struct {
	StateHash                          string           `json:"-" db:"state_hash"`
	WorkspaceID                        string           `json:"workspace_id" db:"workspace_id"`
	UserID                             string           `json:"user_id" db:"user_id"`
	AppRegistrationID                  string           `json:"app_registration_id" db:"app_registration_id"`
	Kind                               AuthFlowKind     `json:"kind" db:"kind"`
	PKCEVerifier                       string           `json:"-" db:"pkce_verifier"`
	ExpectedWorkspaceSource            ConnectionSource `json:"-" db:"expected_workspace_source"`
	ExpectedWorkspaceGeneration        int64            `json:"-" db:"expected_workspace_generation"`
	ExpectedInstallationID             *int64           `json:"-" db:"expected_installation_id"`
	ExpectedWorkspaceAppRegistrationID string           `json:"-" db:"expected_workspace_app_registration_id"`
	ExpectedPersonalGeneration         int64            `json:"-" db:"expected_personal_generation"`
	ExpiresAt                          time.Time        `json:"expires_at" db:"expires_at"`
	ConsumedAt                         *time.Time       `json:"consumed_at,omitempty" db:"consumed_at"`
	CreatedAt                          time.Time        `json:"created_at" db:"created_at"`
}

type WorkspaceConnectionExpectation struct {
	Source               ConnectionSource
	CredentialGeneration int64
	InstallationID       *int64
	AppRegistrationID    string
}

type WebhookDeliveryStatus string

const (
	WebhookDeliveryStatusReceived  WebhookDeliveryStatus = "received"
	WebhookDeliveryStatusProcessed WebhookDeliveryStatus = "processed"
	WebhookDeliveryStatusIgnored   WebhookDeliveryStatus = "ignored"
	WebhookDeliveryStatusFailed    WebhookDeliveryStatus = "failed"
)

// WebhookDelivery records GitHub delivery IDs so webhook state transitions are
// idempotent without persisting payloads or authentication material.
type WebhookDelivery struct {
	AppRegistrationID string                `json:"app_registration_id" db:"app_registration_id"`
	DeliveryID        string                `json:"delivery_id" db:"delivery_id"`
	Event             string                `json:"event" db:"event"`
	Status            WebhookDeliveryStatus `json:"status" db:"status"`
	Result            string                `json:"result,omitempty" db:"result"`
	ReceivedAt        time.Time             `json:"received_at" db:"received_at"`
	ProcessedAt       *time.Time            `json:"processed_at,omitempty" db:"processed_at"`
}

type WebhookDeliveryClaim struct {
	Acquired bool
	Status   WebhookDeliveryStatus
}

// WorkspacePATSecretKey returns the encrypted-secret ID for workspace PAT auth.
func WorkspacePATSecretKey(workspaceID string) string {
	return "github:workspace:" + workspaceID + ":pat"
}

// UserAccessTokenSecretKey returns the encrypted-secret ID for personal access.
func UserAccessTokenSecretKey(workspaceID, userID string) string {
	return "github:user:" + workspaceID + ":" + userID + ":access"
}

// UserRefreshTokenSecretKey returns the encrypted-secret ID for personal refresh.
func UserRefreshTokenSecretKey(workspaceID, userID string) string {
	return "github:user:" + workspaceID + ":" + userID + ":refresh"
}

// TaskCIAutoFixMaxRounds is the server-enforced CI auto-fix loop guard.
const TaskCIAutoFixMaxRounds = 10

// PR represents a GitHub Pull Request.
type PR struct {
	ID                  int64  `json:"id"`
	NodeID              string `json:"node_id"`
	Number              int    `json:"number"`
	Title               string `json:"title"`
	URL                 string `json:"url"`
	HTMLURL             string `json:"html_url"`
	State               string `json:"state"` // open, closed, merged
	HeadBranch          string `json:"head_branch"`
	HeadSHA             string `json:"head_sha"`
	BaseBranch          string `json:"base_branch"`
	AuthorLogin         string `json:"author_login"`
	RepoOwner           string `json:"repo_owner"`
	RepoName            string `json:"repo_name"`
	HeadRepoID          int64  `json:"head_repo_id,omitempty"`
	HeadRepoNodeID      string `json:"head_repo_node_id,omitempty"`
	HeadRepoOwner       string `json:"head_repo_owner,omitempty"`
	HeadRepoName        string `json:"head_repo_name,omitempty"`
	HeadRepoCloneURL    string `json:"head_repo_clone_url,omitempty"`
	BaseRepoID          int64  `json:"base_repo_id,omitempty"`
	BaseRepoOwner       string `json:"base_repo_owner,omitempty"`
	BaseRepoName        string `json:"base_repo_name,omitempty"`
	BaseDefaultBranch   string `json:"base_default_branch,omitempty"`
	MaintainerCanModify bool   `json:"maintainer_can_modify"`
	Body                string `json:"body"`
	Draft               bool   `json:"draft"`
	Mergeable           bool   `json:"mergeable"`
	MergeableState      string `json:"mergeable_state"` // clean, blocked, behind, dirty, has_hooks, unstable, draft, unknown, ""
	// The mock provider uses these optional fields to reproduce GraphQL merge
	// queue observations through its REST-shaped status path. Production REST
	// payloads leave them empty; GraphQL remains the authoritative queue source.
	MergeQueueState                       string              `json:"merge_queue_state,omitempty"`
	MergeQueuePosition                    *int                `json:"merge_queue_position,omitempty"`
	MergeQueueEntryID                     string              `json:"merge_queue_entry_id,omitempty"`
	MergeQueueEntryHeadSHA                string              `json:"merge_queue_entry_head_sha,omitempty"`
	MergeQueueEstimatedTimeToMergeSeconds *int                `json:"merge_queue_estimated_time_to_merge_seconds,omitempty"`
	MergeQueueLastRemovalID               string              `json:"merge_queue_last_removal_id,omitempty"`
	MergeQueueLastRemovedAt               *time.Time          `json:"merge_queue_last_removed_at,omitempty"`
	MergeQueueLastRemovalReason           string              `json:"merge_queue_last_removal_reason,omitempty"`
	MergeQueueLastRemovalBeforeSHA        string              `json:"merge_queue_last_removal_before_sha,omitempty"`
	Additions                             int                 `json:"additions"`
	Deletions                             int                 `json:"deletions"`
	RequestedReviewers                    []RequestedReviewer `json:"requested_reviewers"`
	CreatedAt                             time.Time           `json:"created_at"`
	UpdatedAt                             time.Time           `json:"updated_at"`
	MergedAt                              *time.Time          `json:"merged_at,omitempty"`
	ClosedAt                              *time.Time          `json:"closed_at,omitempty"`
	// ChangedFiles is the number of files touched by the PR. 0 is a real
	// observation, distinct from "never observed" — see ChangedFilesObserved,
	// which is what actually carries that distinction (AC-12a).
	ChangedFiles int `json:"changed_files,omitempty"`
	// MergedByLogin is "" when the PR was never merged, or upstream reported
	// no merger. Callers that persist this must write NULL for "", never "".
	MergedByLogin string `json:"merged_by_login,omitempty"`
	// AutoMergeEnabled reports whether upstream observed auto-merge armed at
	// fetch time. GitHub clears auto_merge once it fires, so this can only
	// ever mean "armed at this instant" — never "merged by auto-merge".
	AutoMergeEnabled bool `json:"auto_merge_enabled,omitempty"`
	// IsDraftObserved reports whether Draft was decoded from an explicit,
	// non-null upstream isDraft/draft value, as opposed to json.Unmarshal
	// defaulting an absent or null field to false. Internal bookkeeping only
	// (json:"-"): AC-12a requires resolveTaskPROutcomeFields to write NULL
	// for is_draft when this is false, rather than persisting a fabricated
	// "false".
	IsDraftObserved bool `json:"-"`
	// ChangedFilesObserved mirrors IsDraftObserved for ChangedFiles. It is
	// what actually distinguishes a genuine "0 files changed" observation
	// from "changedFiles was absent or null" (AC-12a) — ChangedFiles alone
	// cannot, since both cases decode to the Go zero value.
	ChangedFilesObserved bool `json:"-"`
	// These flags are only used by the in-memory E2E provider to distinguish an
	// observed empty queue entry/event from a REST payload with no queue data.
	mergeQueuePopulated         bool `json:"-"`
	mergeQueueRecoveryPopulated bool `json:"-"`
}

// RequestedReviewer represents a pending reviewer request on a PR.
type RequestedReviewer struct {
	Login string `json:"login"`
	Type  string `json:"type"` // user, team
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
	AuthorIsBot  bool      `json:"author_is_bot"`
	Body         string    `json:"body"`
	Path         string    `json:"path"`
	Line         int       `json:"line"`
	Side         string    `json:"side"` // LEFT, RIGHT
	CommentType  string    `json:"comment_type"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	InReplyTo    *int64    `json:"in_reply_to,omitempty"`
}

// CheckRun represents a CI check result.
type CheckRun struct {
	Name        string     `json:"name"`
	Source      string     `json:"source"`     // check_run, status_context
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

// PRStatus contains lightweight PR state used by the background poller.
// Unlike PRFeedback, it skips comments to reduce API calls.
type PRStatus struct {
	PR                     *PR    `json:"pr"`
	ReviewState            string `json:"review_state"`    // "approved", "changes_requested", "pending", ""
	ChecksState            string `json:"checks_state"`    // "success", "failure", "pending", ""
	MergeableState         string `json:"mergeable_state"` // "clean", "blocked", "behind", "dirty", "has_hooks", "unstable", "draft", "unknown", ""
	MergeQueueState        string `json:"merge_queue_state"`
	MergeQueuePosition     *int   `json:"merge_queue_position,omitempty"`
	MergeQueueEntryID      string `json:"merge_queue_entry_id,omitempty"`
	MergeQueueEntryHeadSHA string `json:"merge_queue_entry_head_sha,omitempty"`
	// A nil estimate is an observed absence when mergeQueuePopulated is true.
	MergeQueueEstimatedTimeToMergeSeconds *int       `json:"merge_queue_estimated_time_to_merge_seconds,omitempty"`
	MergeQueueLastRemovalID               string     `json:"merge_queue_last_removal_id,omitempty"`
	MergeQueueLastRemovedAt               *time.Time `json:"merge_queue_last_removed_at,omitempty"`
	MergeQueueLastRemovalReason           string     `json:"merge_queue_last_removal_reason,omitempty"`
	MergeQueueLastRemovalBeforeSHA        string     `json:"merge_queue_last_removal_before_sha,omitempty"`
	ReviewCount                           int        `json:"review_count"`
	PendingReviewCount                    int        `json:"pending_review_count"`
	RequiredReviews                       *int       `json:"required_reviews,omitempty"` // nil when no branch protection rule found
	ChecksTotal                           int        `json:"checks_total"`
	ChecksPassing                         int        `json:"checks_passing"`
	// ChecksPopulated reports whether the sync path actually computed
	// ChecksTotal / ChecksPassing. The batched GraphQL poller doesn't (it
	// only carries the rollup state), so SyncTaskPR uses this flag to
	// decide whether to overwrite the persisted counts. A value of true
	// with both counts at 0 is a real "no checks" answer; a value of false
	// means "I didn't look, keep what's there."
	ChecksPopulated         bool `json:"checks_populated,omitempty"`
	UnresolvedReviewThreads int  `json:"unresolved_review_threads"`
	// UnresolvedReviewThreadsPopulated mirrors ChecksPopulated for the
	// review-threads field. The REST path (getPRStatus) doesn't fetch
	// review threads, so it leaves the field at zero; SyncTaskPR uses
	// this flag to avoid clobbering a non-zero value set by the GraphQL
	// path during a subsequent REST sync.
	UnresolvedReviewThreadsPopulated bool `json:"unresolved_review_threads_populated,omitempty"`
	// ReviewCountsPopulated covers ReviewCount + PendingReviewCount. Both
	// REST and GraphQL paths compute these now, but historically only the
	// REST path did, so without the guard a GraphQL poll would clobber
	// the REST value back to 0 (the popover's "Approved (1)" turning
	// into "Approved (0)" until a new REST call landed).
	ReviewCountsPopulated bool `json:"review_counts_populated,omitempty"`
	// OutcomeFieldsPopulated mirrors ChecksPopulated for is_draft,
	// changed_files, and merged_by_login: set only by sync paths that fetched
	// a full single pull request or a batched GraphQL result, so a zero/empty
	// value is a real observation rather than "I didn't look."
	OutcomeFieldsPopulated bool `json:"outcome_fields_populated,omitempty"`
	// ClosedByLogin is the closed-event actor's login, populated only by the
	// GraphQL path (closed_by is absent from the REST pulls endpoint and the
	// gh CLI's PR field set).
	ClosedByLogin string `json:"closed_by_login,omitempty"`
	// ClosureAttributionPopulated is set only when a GraphQL sync observed a
	// closed-event actor with a non-empty login. REST and gh CLI single-PR
	// syncs can never set this — they have no closing-actor field to read.
	ClosureAttributionPopulated bool `json:"closure_attribution_populated,omitempty"`
	// mergeQueuePopulated distinguishes a GraphQL observation (including a
	// null mergeQueueEntry) from REST and gh CLI reads that have no queue data.
	mergeQueuePopulated bool
	// mergeQueueRecoveryPopulated distinguishes a GraphQL observation of the
	// latest removal event (including no event) from REST and gh CLI reads.
	mergeQueueRecoveryPopulated bool
}

// PRSearchPage is a paginated slice of PR search results, with the total
// count reported by the GitHub Search API (capped at 1000).
type PRSearchPage struct {
	PRs        []*PR `json:"prs"`
	TotalCount int   `json:"total_count"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
}

// IssueSearchPage is a paginated slice of Issue search results.
type IssueSearchPage struct {
	Issues     []*Issue `json:"issues"`
	TotalCount int      `json:"total_count"`
	Page       int      `json:"page"`
	PerPage    int      `json:"per_page"`
}

// PRWatch tracks active PR monitoring (session → PR). RepositoryID identifies
// which task repository the watched PR belongs to (multi-repo support; empty
// for legacy rows).
type PRWatch struct {
	ID              string     `json:"id" db:"id"`
	WorkspaceID     string     `json:"workspace_id" db:"workspace_id"`
	SessionID       string     `json:"session_id" db:"session_id"`
	TaskID          string     `json:"task_id" db:"task_id"`
	RepositoryID    string     `json:"repository_id,omitempty" db:"repository_id"`
	Owner           string     `json:"owner" db:"owner"`
	Repo            string     `json:"repo" db:"repo"`
	PRNumber        int        `json:"pr_number" db:"pr_number"`
	Branch          string     `json:"branch" db:"branch"`
	LastCheckedAt   *time.Time `json:"last_checked_at,omitempty" db:"last_checked_at"`
	LastCommentAt   *time.Time `json:"last_comment_at,omitempty" db:"last_comment_at"`
	LastCheckStatus string     `json:"last_check_status" db:"last_check_status"`
	LastReviewState string     `json:"last_review_state" db:"last_review_state"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskPR associates a PR with a task. RepositoryID identifies which task
// repository this PR belongs to (multi-repo tasks can have one PR per repo).
// Empty for legacy rows persisted before multi-repo support.
type TaskPR struct {
	ID                                    string     `json:"id" db:"id"`
	WorkspaceID                           string     `json:"workspace_id" db:"workspace_id"`
	TaskID                                string     `json:"task_id" db:"task_id"`
	RepositoryID                          string     `json:"repository_id,omitempty" db:"repository_id"`
	Owner                                 string     `json:"owner" db:"owner"`
	Repo                                  string     `json:"repo" db:"repo"`
	PRNumber                              int        `json:"pr_number" db:"pr_number"`
	PRURL                                 string     `json:"pr_url" db:"pr_url"`
	PRTitle                               string     `json:"pr_title" db:"pr_title"`
	HeadBranch                            string     `json:"head_branch" db:"head_branch"`
	BaseBranch                            string     `json:"base_branch" db:"base_branch"`
	AuthorLogin                           string     `json:"author_login" db:"author_login"`
	State                                 string     `json:"state" db:"state"`                     // open, closed, merged
	ReviewState                           string     `json:"review_state" db:"review_state"`       // approved, changes_requested, pending, ""
	ChecksState                           string     `json:"checks_state" db:"checks_state"`       // success, failure, pending, ""
	MergeableState                        string     `json:"mergeable_state" db:"mergeable_state"` // clean, blocked, behind, dirty, has_hooks, unstable, draft, unknown, ""
	HeadSHA                               string     `json:"head_sha" db:"head_sha"`
	MergeQueueState                       string     `json:"merge_queue_state" db:"merge_queue_state"`
	MergeQueuePosition                    *int       `json:"merge_queue_position" db:"merge_queue_position"`
	MergeQueueEntryID                     string     `json:"merge_queue_entry_id" db:"merge_queue_entry_id"`
	MergeQueueEntryHeadSHA                string     `json:"merge_queue_entry_head_sha" db:"merge_queue_entry_head_sha"`
	MergeQueueEstimatedTimeToMergeSeconds *int       `json:"merge_queue_estimated_time_to_merge_seconds" db:"merge_queue_estimated_time_to_merge_seconds"`
	MergeQueueLastRemovalID               string     `json:"merge_queue_last_removal_id" db:"merge_queue_last_removal_id"`
	MergeQueueLastRemovedAt               *time.Time `json:"merge_queue_last_removed_at,omitempty" db:"merge_queue_last_removed_at"`
	MergeQueueLastRemovalReason           string     `json:"merge_queue_last_removal_reason" db:"merge_queue_last_removal_reason"`
	MergeQueueLastRemovalBeforeSHA        string     `json:"merge_queue_last_removal_before_sha" db:"merge_queue_last_removal_before_sha"`
	ReviewCount                           int        `json:"review_count" db:"review_count"`
	PendingReviewCount                    int        `json:"pending_review_count" db:"pending_review_count"`
	// RequiredReviews is the branch protection's required_approving_review_count.
	// Nil when no protection rule exists or the token lacks scope to read it.
	RequiredReviews         *int       `json:"required_reviews,omitempty" db:"required_reviews"`
	CommentCount            int        `json:"comment_count" db:"comment_count"`
	UnresolvedReviewThreads int        `json:"unresolved_review_threads" db:"unresolved_review_threads"`
	ChecksTotal             int        `json:"checks_total" db:"checks_total"`
	ChecksPassing           int        `json:"checks_passing" db:"checks_passing"`
	Additions               int        `json:"additions" db:"additions"`
	Deletions               int        `json:"deletions" db:"deletions"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	MergedAt                *time.Time `json:"merged_at,omitempty" db:"merged_at"`
	ClosedAt                *time.Time `json:"closed_at,omitempty" db:"closed_at"`
	LastSyncedAt            *time.Time `json:"last_synced_at,omitempty" db:"last_synced_at"`
	DetachedAt              *time.Time `json:"-" db:"detached_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`

	// --- PR outcome attribution (five nullable columns, never backfilled) ---
	//
	// Every field below is NULL on any row that predates this feature's
	// activation instant (kandev_meta key taskPROutcomeActivatedAtMetaKey)
	// and stays NULL until a post-activation observation writes it. None has a
	// non-NULL default, and none is ever
	// inferred, backfilled, or defaulted to a zero value. No `omitempty` on
	// any of these json tags: AC-30 requires the keys to always be present,
	// because `null` vs. absent is exactly the distinction this feature
	// exists to preserve.
	//
	// Writer-health invariants (auditable, not a dashboard):
	//   - AC-36: for any row where merged_at >= activation, MergedByLogin
	//     must be non-NULL. merged_at is only ever written by the sync
	//     writer, so a row that merged after activation was necessarily
	//     observed by a post-activation writer; a NULL there is a writer
	//     fault, not a data gap.
	//   - AC-37: for any row where last_synced_at >= activation, IsDraft
	//     must be non-NULL whenever the row's most recent sync was a
	//     populating one. IsDraft is supplied by every populating sync path,
	//     so it is the primary canary for "the writer stopped."
	//   - AC-39: neither invariant applies to rows whose merged_at /
	//     closed_at predates the activation instant — those rows are
	//     legitimately and permanently NULL.

	// IsDraft is never observed by a populating sync when NULL.
	IsDraft *bool `json:"is_draft" db:"is_draft"`
	// ChangedFiles is never observed when NULL, distinct from 0 (a real
	// "no files changed" observation).
	ChangedFiles *int `json:"changed_files" db:"changed_files"`
	// MergedByLogin is NULL when the PR was never merged, or merged but
	// never observed by a populating sync.
	MergedByLogin *string `json:"merged_by_login" db:"merged_by_login"`
	// ClosedByLogin is NULL when the PR was never closed, or closure was
	// never observed by the GraphQL path specifically. GitHub's closed_by
	// is absent from the REST pulls endpoint and from the gh CLI's PR field
	// set (only the issues endpoint carries it), so this column is sourced
	// from a GraphQL closed-event actor selection only. A PR whose only
	// post-closure sync came through REST or gh CLI keeps this NULL
	// permanently, because terminal rows are excluded from the orphan sweep
	// (service_pr_unwatched.go). This gap is accepted (AC-15) and must be
	// stated wherever this column is consumed.
	ClosedByLogin *string `json:"closed_by_login" db:"closed_by_login"`
	// AutoMergeObservedAt is a latched observation, never a merge cause.
	// GitHub clears `auto_merge` once it fires, so a poller can only ever
	// learn "auto-merge was armed at some instant while we were looking."
	// It is set once (the first time armed auto-merge is observed) and is
	// never cleared or overwritten afterwards, including when a later sync
	// observes auto-merge disarmed or absent. It must not be read, named,
	// or charted as "merged by auto-merge."
	AutoMergeObservedAt *time.Time `json:"auto_merge_observed_at" db:"auto_merge_observed_at"`
}

// GetWorkspaceID lets workspace-scoped notification handlers route the typed
// in-process task PR event to the owning workspace instead of broadcasting it
// as an unattributed instance-wide update.
func (p TaskPR) GetWorkspaceID() string { return p.WorkspaceID }

// TaskCIOptions stores task-level PR automation preferences.
//
// The five automation switches below (AutoFixEnabled, AutoMergeEnabled,
// PromptOnReviewRequested, PromptOnMerged, PromptOnClosed) are legacy: they
// are no longer written by UpdateTaskCIOptions and are read only by the
// one-time pr_scope_migrated_at fan-out migration
// (migrateTaskCIOptionsToPRScope). The per-PR source of truth is
// TaskPRAutomationOptions / github_task_pr_automation_options. Genuinely
// task-level fields (AutoFixPromptOverride, ReviewReviewerLogin) remain here.
type TaskCIOptions struct {
	TaskID                  string  `json:"task_id" db:"task_id"`
	AutoFixEnabled          bool    `json:"-" db:"auto_fix_enabled"`
	AutoMergeEnabled        bool    `json:"-" db:"auto_merge_enabled"`
	AutoFixPromptOverride   *string `json:"auto_fix_prompt_override,omitempty" db:"auto_fix_prompt_override"`
	PromptOnReviewRequested bool    `json:"-" db:"prompt_on_review_requested"`
	PromptOnMerged          bool    `json:"-" db:"prompt_on_merged"`
	PromptOnClosed          bool    `json:"-" db:"prompt_on_closed"`
	ReviewReviewerLogin     string  `json:"review_reviewer_login" db:"review_reviewer_login"`
	// Lifecycle override columns remain only to read and clear legacy rows during
	// the additive schema migration. They are not part of the update or API model.
	ReviewPromptOverride *string `json:"-" db:"review_prompt_override"`
	MergedPromptOverride *string `json:"-" db:"merged_prompt_override"`
	ClosedPromptOverride *string `json:"-" db:"closed_prompt_override"`
	// PRScopeMigratedAt guards the one-time fan-out of the legacy booleans
	// above onto github_task_pr_automation_options. Nil means "not yet
	// migrated"; non-nil means the fan-out already ran for this task and
	// must never re-run (it would silently re-enable a switch a user has
	// since turned off for a specific PR).
	PRScopeMigratedAt *time.Time `json:"-" db:"pr_scope_migrated_at"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskCIOptionsPatch is a partial update for task CI automation options.
// RepositoryID/PRNumber optionally target one linked PR for the five
// automation switches; when both are nil the switches fan out to every PR
// currently linked to the task. AutoFixPromptOverride and ReviewReviewerLogin
// are always task-level regardless of PR identity.
type TaskCIOptionsPatch struct {
	RepositoryID            *string
	PRNumber                *int
	AutoFixEnabled          *bool
	AutoMergeEnabled        *bool
	AutoFixPromptOverride   *string
	PromptOnReviewRequested *bool
	PromptOnMerged          *bool
	PromptOnClosed          *bool
	ReviewReviewerLogin     *string
}

// HasAny reports whether the patch contains at least one requested field change.
func (p TaskCIOptionsPatch) HasAny() bool {
	return p.AutoFixEnabled != nil || p.AutoMergeEnabled != nil || p.AutoFixPromptOverride != nil ||
		p.PromptOnReviewRequested != nil || p.PromptOnMerged != nil || p.PromptOnClosed != nil ||
		p.ReviewReviewerLogin != nil
}

// PRAutomationPatch extracts the per-PR automation switch fields.
func (p TaskCIOptionsPatch) PRAutomationPatch() TaskPRAutomationOptionsPatch {
	return TaskPRAutomationOptionsPatch{
		AutoFixEnabled:          p.AutoFixEnabled,
		AutoMergeEnabled:        p.AutoMergeEnabled,
		PromptOnReviewRequested: p.PromptOnReviewRequested,
		PromptOnMerged:          p.PromptOnMerged,
		PromptOnClosed:          p.PromptOnClosed,
	}
}

// TaskPRAutomationOptions stores per-PR automation switches, keyed by
// (task_id, repository_id, pr_number). This is the source of truth for the
// five CI/lifecycle automation switches; TaskCIOptions keeps only the
// genuinely task-level fields (prompt override, reviewer login).
type TaskPRAutomationOptions struct {
	TaskID                  string    `json:"task_id" db:"task_id"`
	RepositoryID            string    `json:"repository_id" db:"repository_id"`
	PRNumber                int       `json:"pr_number" db:"pr_number"`
	AutoFixEnabled          bool      `json:"auto_fix_enabled" db:"auto_fix_enabled"`
	AutoMergeEnabled        bool      `json:"auto_merge_enabled" db:"auto_merge_enabled"`
	PromptOnReviewRequested bool      `json:"prompt_on_review_requested" db:"prompt_on_review_requested"`
	PromptOnMerged          bool      `json:"prompt_on_merged" db:"prompt_on_merged"`
	PromptOnClosed          bool      `json:"prompt_on_closed" db:"prompt_on_closed"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time `json:"updated_at" db:"updated_at"`
}

// TaskPRAutomationOptionsPatch is a partial update for one PR's automation switches.
type TaskPRAutomationOptionsPatch struct {
	AutoFixEnabled          *bool
	AutoMergeEnabled        *bool
	PromptOnReviewRequested *bool
	PromptOnMerged          *bool
	PromptOnClosed          *bool
}

// HasAny reports whether the patch contains at least one requested field change.
func (p TaskPRAutomationOptionsPatch) HasAny() bool {
	return p.AutoFixEnabled != nil || p.AutoMergeEnabled != nil ||
		p.PromptOnReviewRequested != nil || p.PromptOnMerged != nil || p.PromptOnClosed != nil
}

// TaskCIOptionsResponse is the HTTP shape for task CI automation options.
// The top-level automation booleans are an aggregate over PROptions ("every
// linked PR has this switch enabled, and at least one PR is linked") kept for
// MCP/API read compatibility; PROptions is the per-PR source of truth.
type TaskCIOptionsResponse struct {
	TaskID                  string                     `json:"task_id"`
	AutoFixEnabled          bool                       `json:"auto_fix_enabled"`
	AutoMergeEnabled        bool                       `json:"auto_merge_enabled"`
	AutoFixPromptOverride   *string                    `json:"auto_fix_prompt_override"`
	AutoFixMaxRounds        int                        `json:"auto_fix_max_rounds"`
	EffectiveAutoFixPrompt  string                     `json:"effective_auto_fix_prompt"`
	UsingDefaultPrompt      bool                       `json:"using_default_prompt"`
	PromptOnReviewRequested bool                       `json:"prompt_on_review_requested"`
	PromptOnMerged          bool                       `json:"prompt_on_merged"`
	PromptOnClosed          bool                       `json:"prompt_on_closed"`
	ReviewReviewerLogin     string                     `json:"review_reviewer_login"`
	EffectiveReviewPrompt   string                     `json:"-"`
	EffectiveMergedPrompt   string                     `json:"-"`
	EffectiveClosedPrompt   string                     `json:"-"`
	UpdatedAt               time.Time                  `json:"updated_at"`
	PRStates                []*TaskCIPRAutomationState `json:"pr_states"`
	PROptions               []*TaskPRAutomationOptions `json:"pr_options"`
	// WorkspaceID is routing metadata for workspace-scoped WebSocket delivery.
	// It stays JSON-visible because NATS-backed event buses round-trip payloads.
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// GetWorkspaceID lets the WebSocket broadcaster route CI-option updates to
// the task's owning workspace when the in-memory event retains its typed shape.
func (r *TaskCIOptionsResponse) GetWorkspaceID() string {
	if r == nil {
		return ""
	}
	return r.WorkspaceID
}

// TaskCIPRAutomationState stores per-PR dedupe and error state for CI automation.
type TaskCIPRAutomationState struct {
	TaskID                   string     `json:"task_id" db:"task_id"`
	RepositoryID             string     `json:"repository_id" db:"repository_id"`
	PRNumber                 int        `json:"pr_number" db:"pr_number"`
	LastFixSignature         string     `json:"last_fix_signature" db:"last_fix_signature"`
	LastFixCheckpointJSON    string     `json:"last_fix_checkpoint_json" db:"last_fix_checkpoint_json"`
	LastFixEnqueuedAt        *time.Time `json:"last_fix_enqueued_at,omitempty" db:"last_fix_enqueued_at"`
	LastFixSessionID         *string    `json:"last_fix_session_id,omitempty" db:"last_fix_session_id"`
	AutoFixRoundCount        int        `json:"auto_fix_round_count" db:"auto_fix_round_count"`
	AutoFixExhaustedAt       *time.Time `json:"auto_fix_exhausted_at" db:"auto_fix_exhausted_at"`
	LastMergeSignature       string     `json:"last_merge_signature" db:"last_merge_signature"`
	LastMergeAttemptAt       *time.Time `json:"last_merge_attempt_at,omitempty" db:"last_merge_attempt_at"`
	LastQueueAttemptHeadSHA  string     `json:"last_queue_attempt_head_sha" db:"last_queue_attempt_head_sha"`
	LastQueueFixEventID      string     `json:"last_queue_fix_event_id" db:"last_queue_fix_event_id"`
	LastQueueRemovalCause    string     `json:"last_queue_removal_cause" db:"last_queue_removal_cause"`
	ReviewRequestInitialized bool       `json:"review_request_initialized" db:"review_request_initialized"`
	LastReviewRequested      bool       `json:"last_review_requested" db:"last_review_requested"`
	LastObservedPRState      string     `json:"last_observed_pr_state" db:"last_observed_pr_state"`
	LastLifecycleEvent       string     `json:"last_lifecycle_event" db:"last_lifecycle_event"`
	LastLifecyclePromptAt    *time.Time `json:"last_lifecycle_prompt_at,omitempty" db:"last_lifecycle_prompt_at"`
	LastLifecycleSessionID   *string    `json:"last_lifecycle_session_id,omitempty" db:"last_lifecycle_session_id"`
	LastError                *string    `json:"last_error,omitempty" db:"last_error"`
	CreatedAt                time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskCIFixAttempt records an auto-fix prompt attempt for a task PR.
type TaskCIFixAttempt struct {
	TaskID              string
	RepositoryID        string
	PRNumber            int
	Signature           string
	CheckpointJSON      string
	SessionID           string
	EnqueuedAt          time.Time
	IncrementRound      bool
	QueueRemovalEventID string
	QueueRemovalCause   string
}

// TaskCIMergeAttempt records an auto-merge attempt for a task PR.
type TaskCIMergeAttempt struct {
	TaskID           string
	RepositoryID     string
	PRNumber         int
	Signature        string
	AttemptedAt      time.Time
	AttemptedHeadSHA string
}

// TaskCIMergeQueueObservation records queue membership and removal evidence
// observed by the PR status poller so automation survives process restarts.
type TaskCIMergeQueueObservation struct {
	TaskID                 string
	RepositoryID           string
	PRNumber               int
	ActiveQueueHeadSHA     string
	MergeSignature         string
	RemovalEventID         string
	RemovalCause           string
	RemovalObservedHeadSHA string
}

// TaskPRLifecyclePrompt records an accepted lifecycle prompt checkpoint.
type TaskPRLifecyclePrompt struct {
	TaskID          string
	RepositoryID    string
	PRNumber        int
	Event           string
	SessionID       string
	PromptedAt      time.Time
	ReviewRequested bool
	ObservedState   string
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

// CleanupPolicy controls how a review or issue watch handles its auto-created
// tasks once the underlying PR / issue reaches a terminal state.
const (
	// CleanupPolicyAuto deletes the task once the PR/issue is merged or closed
	// UNLESS the user authored at least one message in the task (the agent's
	// auto-start prompt does not count).
	CleanupPolicyAuto = "auto"
	// CleanupPolicyAlways deletes the task on terminal state regardless of
	// user interaction. Use when the watch is purely informational and the
	// user never expects a banner / history for merged PRs.
	CleanupPolicyAlways = "always"
	// CleanupPolicyNever disables automatic cleanup. Tasks pile up until the
	// user invokes manual cleanup or deletes them by hand.
	CleanupPolicyNever = "never"
)

// IsValidCleanupPolicy reports whether s is one of the recognized policies.
// Empty string is treated as valid so legacy rows (pre-migration) and zero
// values default to "auto" downstream.
func IsValidCleanupPolicy(s string) bool {
	switch s {
	case "", CleanupPolicyAuto, CleanupPolicyAlways, CleanupPolicyNever:
		return true
	}
	return false
}

// NormalizeCleanupPolicy maps the empty string to CleanupPolicyAuto. Unknown
// values are returned unchanged so the caller can surface a validation error.
func NormalizeCleanupPolicy(s string) string {
	if s == "" {
		return CleanupPolicyAuto
	}
	return s
}

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
	TargetLogin         string       `json:"target_login" db:"target_login"`
	Enabled             bool         `json:"enabled" db:"enabled"`
	PollIntervalSeconds int          `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	CleanupPolicy       string       `json:"cleanup_policy" db:"cleanup_policy"`
	LastPolledAt        *time.Time   `json:"last_polled_at,omitempty" db:"last_polled_at"`
	// LastError / LastErrorAt are stamped when the dispatch pipeline self-
	// heals the watcher (e.g. the bound agent profile was soft-deleted).
	LastError   string     `json:"last_error,omitempty" db:"last_error"`
	LastErrorAt *time.Time `json:"last_error_at,omitempty" db:"last_error_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
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
// PushedAt is the timestamp of the latest push, used to sort the
// list-accessible-repos result (most-recently-active first). It is a pointer
// so `omitempty` actually drops it from JSON when unset — a zero `time.Time`
// struct would otherwise serialize as `"0001-01-01T00:00:00Z"`.
//
// DefaultBranch lets the Remote picker pre-fill the row's branch immediately
// on selection (skips the branch-list round-trip for the common case). The
// GitHub API returns it on every repo, so it is required. Description is
// omitempty because GitHub may return null/empty.
type GitHubRepo struct {
	FullName      string     `json:"full_name"`
	Owner         string     `json:"owner"`
	Name          string     `json:"name"`
	Private       bool       `json:"private"`
	DefaultBranch string     `json:"default_branch"`
	Description   string     `json:"description,omitempty"`
	PushedAt      *time.Time `json:"pushed_at,omitempty"`
}

// GitHubRepository is the provider-authoritative identity and permission
// projection needed when preparing a managed contribution destination. The
// derived parent and permission fields deliberately do not carry raw API
// response shapes into task metadata.
type GitHubRepository struct {
	ID             int64
	NodeID         string
	FullName       string
	Owner          string
	Name           string
	CloneURL       string
	HTMLURL        string
	DefaultBranch  string
	Fork           bool
	ParentID       int64
	ParentFullName string
	PushAccess     bool
	AdminAccess    bool
}

// RepoBranch represents a branch in a GitHub repository.
type RepoBranch struct {
	Name string `json:"name"`
}

// Repo content entry types, as reported by the GitHub contents API's "type" field.
const (
	RepoContentTypeFile = "file"
	RepoContentTypeDir  = "dir"
)

// RepoContentEntry describes one entry returned when listing a repository directory.
type RepoContentEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "file" or "dir"
	SHA  string `json:"sha"`
	Size int    `json:"size"`
}

// RepoMergeMethods reports which merge methods a repository allows. Mirrors
// the allow_*_merge booleans from GET /repos/{owner}/{repo}.
type RepoMergeMethods struct {
	Merge  bool `json:"merge"`
	Squash bool `json:"squash"`
	Rebase bool `json:"rebase"`
}

// GitHubStatus represents GitHub connection status.
type GitHubStatus struct {
	Authenticated   bool                 `json:"authenticated"`
	Username        string               `json:"username"`
	AuthMethod      string               `json:"auth_method"` // "gh_cli", "pat", "none"
	TokenConfigured bool                 `json:"token_configured"`
	TokenSecretID   string               `json:"token_secret_id,omitempty"`
	RequiredScopes  []string             `json:"required_scopes"`
	Diagnostics     *AuthDiagnostics     `json:"diagnostics,omitempty"`
	RateLimit       *GitHubRateLimitInfo `json:"rate_limit,omitempty"`
}

// GitHubRateLimitInfo bundles known rate-limit snapshots per resource bucket
// for surfacing in the UI.
type GitHubRateLimitInfo struct {
	Core    *RateSnapshot `json:"core,omitempty"`
	GraphQL *RateSnapshot `json:"graphql,omitempty"`
	Search  *RateSnapshot `json:"search,omitempty"`
}

// ConfigureTokenRequest is the request body for configuring a GitHub token.
type ConfigureTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// AuthDiagnostics captures the output of gh auth status for troubleshooting.
type AuthDiagnostics struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
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
	CleanupPolicy       string       `json:"cleanup_policy"`
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
	CleanupPolicy       *string       `json:"cleanup_policy,omitempty"`
}

// PRFeedbackEvent is published to the event bus when a PR has new feedback.
type PRFeedbackEvent struct {
	SessionID      string `json:"session_id"`
	TaskID         string `json:"task_id"`
	PRNumber       int    `json:"pr_number"`
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
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

// PRFile represents a file changed in a pull request.
type PRFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"` // added, removed, modified, renamed, copied, changed, unchanged
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
	OldPath   string `json:"old_path,omitempty"`
}

// PRCommitInfo represents a commit in a pull request.
type PRCommitInfo struct {
	SHA            string `json:"sha"`
	Message        string `json:"message"`
	AuthorLogin    string `json:"author_login"`
	AuthorDate     string `json:"author_date"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	FilesChanged   int    `json:"files_changed"`
	StatsAvailable bool   `json:"stats_available"`
}

// PRCommitsResult contains the current provider head and the ancestry data
// used to classify a contribution checkout. Complete is true only when the
// provider client finished loading the full PR commit list.
type PRCommitsResult struct {
	Commits  []PRCommitInfo `json:"commits"`
	HeadSHA  string         `json:"head_sha"`
	Complete bool           `json:"complete"`
}

// PRCommitDetail represents the metadata and changed files for one GitHub
// commit. Unlike PRCommitInfo, its statistics come from GitHub's individual
// commit endpoint and are always measured.
type PRCommitDetail struct {
	SHA          string   `json:"sha"`
	Message      string   `json:"message"`
	AuthorLogin  string   `json:"author_login"`
	AuthorName   string   `json:"author_name"`
	AuthorDate   string   `json:"author_date"`
	Additions    int      `json:"additions"`
	Deletions    int      `json:"deletions"`
	FilesChanged int      `json:"files_changed"`
	Files        []PRFile `json:"files"`
}

// DailyCount holds a date and count for chart data.
type DailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// --- Issue Watch models ---

// Issue represents a GitHub Issue (not a PR).
type Issue struct {
	ID          int64      `json:"id"`
	NodeID      string     `json:"node_id"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	URL         string     `json:"url"`
	HTMLURL     string     `json:"html_url"`
	State       string     `json:"state"` // open, closed
	AuthorLogin string     `json:"author_login"`
	RepoOwner   string     `json:"repo_owner"`
	RepoName    string     `json:"repo_name"`
	Labels      []string   `json:"labels"`
	Assignees   []string   `json:"assignees"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

// IssueWatch configures periodic polling for GitHub issues matching a query.
// Repos holds the list of repositories to monitor. An empty list means all repos.
type IssueWatch struct {
	ID                  string       `json:"id" db:"id"`
	WorkspaceID         string       `json:"workspace_id" db:"workspace_id"`
	WorkflowID          string       `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID      string       `json:"workflow_step_id" db:"workflow_step_id"`
	Repos               []RepoFilter `json:"repos" db:"-"`
	ReposJSON           string       `json:"-" db:"repos"`
	AgentProfileID      string       `json:"agent_profile_id" db:"agent_profile_id"`
	ExecutorProfileID   string       `json:"executor_profile_id" db:"executor_profile_id"`
	Prompt              string       `json:"prompt" db:"prompt"`
	Labels              []string     `json:"labels" db:"-"`
	LabelsJSON          string       `json:"-" db:"labels"`
	CustomQuery         string       `json:"custom_query" db:"custom_query"`
	Enabled             bool         `json:"enabled" db:"enabled"`
	PollIntervalSeconds int          `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	CleanupPolicy       string       `json:"cleanup_policy" db:"cleanup_policy"`
	LastPolledAt        *time.Time   `json:"last_polled_at,omitempty" db:"last_polled_at"`
	// LastError / LastErrorAt are stamped when the dispatch pipeline self-
	// heals the watcher (e.g. the bound agent profile was soft-deleted).
	LastError   string     `json:"last_error,omitempty" db:"last_error"`
	LastErrorAt *time.Time `json:"last_error_at,omitempty" db:"last_error_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// IssueWatchTask records which issues have already had tasks created (deduplication).
type IssueWatchTask struct {
	ID           string    `json:"id" db:"id"`
	IssueWatchID string    `json:"issue_watch_id" db:"issue_watch_id"`
	RepoOwner    string    `json:"repo_owner" db:"repo_owner"`
	RepoName     string    `json:"repo_name" db:"repo_name"`
	IssueNumber  int       `json:"issue_number" db:"issue_number"`
	IssueURL     string    `json:"issue_url" db:"issue_url"`
	TaskID       string    `json:"task_id" db:"task_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// NewIssueEvent is published when a new issue matching a watch is found.
type NewIssueEvent struct {
	IssueWatchID      string `json:"issue_watch_id"`
	WorkspaceID       string `json:"workspace_id"`
	WorkflowID        string `json:"workflow_id"`
	WorkflowStepID    string `json:"workflow_step_id"`
	AgentProfileID    string `json:"agent_profile_id"`
	ExecutorProfileID string `json:"executor_profile_id"`
	Prompt            string `json:"prompt"`
	Issue             *Issue `json:"issue"`
}

// CreateIssueWatchRequest is the request body for creating an issue watch.
type CreateIssueWatchRequest struct {
	WorkspaceID         string       `json:"workspace_id"`
	WorkflowID          string       `json:"workflow_id"`
	WorkflowStepID      string       `json:"workflow_step_id"`
	Repos               []RepoFilter `json:"repos"`
	AgentProfileID      string       `json:"agent_profile_id"`
	ExecutorProfileID   string       `json:"executor_profile_id"`
	Prompt              string       `json:"prompt"`
	Labels              []string     `json:"labels"`
	CustomQuery         string       `json:"custom_query"`
	PollIntervalSeconds int          `json:"poll_interval_seconds"`
	CleanupPolicy       string       `json:"cleanup_policy"`
}

// UpdateIssueWatchRequest is the request body for updating an issue watch.
type UpdateIssueWatchRequest struct {
	WorkflowID          *string       `json:"workflow_id,omitempty"`
	WorkflowStepID      *string       `json:"workflow_step_id,omitempty"`
	Repos               *[]RepoFilter `json:"repos,omitempty"`
	AgentProfileID      *string       `json:"agent_profile_id,omitempty"`
	ExecutorProfileID   *string       `json:"executor_profile_id,omitempty"`
	Prompt              *string       `json:"prompt,omitempty"`
	Labels              *[]string     `json:"labels,omitempty"`
	CustomQuery         *string       `json:"custom_query,omitempty"`
	Enabled             *bool         `json:"enabled,omitempty"`
	PollIntervalSeconds *int          `json:"poll_interval_seconds,omitempty"`
	CleanupPolicy       *string       `json:"cleanup_policy,omitempty"`
}

// --- Workspace GitHub scope/settings ---

const (
	RepoScopeModeAll               = "all"
	RepoScopeModeOrgs              = "orgs"
	RepoScopeModeRepos             = "repos"
	TaskGitCredentialsModeManaged  = "managed"
	TaskGitCredentialsModeExecutor = "executor"
)

// WorkspaceSettings stores per-workspace GitHub operational settings.
// Authentication is stored separately and is intentionally not copied with
// these operational settings.
type WorkspaceSettings struct {
	WorkspaceID            string          `json:"workspace_id" db:"workspace_id"`
	TaskGitCredentialsMode string          `json:"task_git_credentials_mode" db:"task_git_credentials_mode"`
	RepoScopeMode          string          `json:"repo_scope_mode" db:"repo_scope_mode"`
	RepoScopeOrgs          []string        `json:"repo_scope_orgs" db:"-"`
	RepoScopeRepos         []RepoFilter    `json:"repo_scope_repos" db:"-"`
	RepoScopeOrgsJSON      string          `json:"-" db:"repo_scope_orgs"`
	RepoScopeReposJSON     string          `json:"-" db:"repo_scope_repos"`
	SavedPresets           json.RawMessage `json:"saved_presets,omitempty" db:"saved_presets"`
	DefaultQueryPresets    json.RawMessage `json:"default_query_presets,omitempty" db:"default_query_presets"`
	CreatedAt              time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at" db:"updated_at"`
}

// UpdateWorkspaceSettingsRequest replaces the workspace GitHub scope and/or
// preference blobs. Nil blobs leave values unchanged; explicit JSON null clears
// default query presets back to built-ins.
type UpdateWorkspaceSettingsRequest struct {
	WorkspaceID            string           `json:"workspace_id"`
	TaskGitCredentialsMode *string          `json:"task_git_credentials_mode,omitempty"`
	RepoScopeMode          *string          `json:"repo_scope_mode,omitempty"`
	RepoScopeOrgs          *[]string        `json:"repo_scope_orgs,omitempty"`
	RepoScopeRepos         *[]RepoFilter    `json:"repo_scope_repos,omitempty"`
	SavedPresets           *json.RawMessage `json:"saved_presets,omitempty"`
	DefaultQueryPresets    *json.RawMessage `json:"default_query_presets,omitempty"`
	SavedPresetsSet        bool             `json:"-"`
	DefaultQueriesSet      bool             `json:"-"`
}

func (r *UpdateWorkspaceSettingsRequest) UnmarshalJSON(data []byte) error {
	type alias UpdateWorkspaceSettingsRequest
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = UpdateWorkspaceSettingsRequest(decoded)
	if value, ok := raw["saved_presets"]; ok {
		r.SavedPresetsSet = true
		if string(value) == jsonNullLiteral {
			r.SavedPresets = nil
		} else {
			next := cloneRawMessage(value)
			r.SavedPresets = &next
		}
	}
	if value, ok := raw["default_query_presets"]; ok {
		r.DefaultQueriesSet = true
		if string(value) == jsonNullLiteral {
			r.DefaultQueryPresets = nil
		} else {
			next := cloneRawMessage(value)
			r.DefaultQueryPresets = &next
		}
	}
	return nil
}

// --- Action presets (quick-launch prompts on the /github page) ---

// ActionPresetKind enumerates the two lists of quick-launch presets.
const (
	ActionPresetKindPR    = "pr"
	ActionPresetKindIssue = "issue"
)

// ActionPreset is a single configurable quick-task launcher entry.
// PromptTemplate supports `{{url}}` and `{{title}}` placeholders which are
// substituted client-side when the dialog is opened.
type ActionPreset struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Hint           string `json:"hint"`
	Icon           string `json:"icon"`
	PromptTemplate string `json:"prompt_template"`
}

// ActionPresets groups the PR and Issue preset lists for a workspace.
type ActionPresets struct {
	WorkspaceID string         `json:"workspace_id"`
	PR          []ActionPreset `json:"pr"`
	Issue       []ActionPreset `json:"issue"`
}

// UpdateActionPresetsRequest replaces one or both preset lists for a workspace.
// Nil fields are left unchanged.
type UpdateActionPresetsRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	PR          *[]ActionPreset `json:"pr,omitempty"`
	Issue       *[]ActionPreset `json:"issue,omitempty"`
}

// DefaultPRActionPresets returns the built-in PR presets used when a workspace
// has no stored overrides.
func DefaultPRActionPresets() []ActionPreset {
	return []ActionPreset{
		{
			ID:             "review",
			Label:          "Review",
			Hint:           "Read the diff, flag issues",
			Icon:           "eye",
			PromptTemplate: "Review the pull request at {{url}}. Provide feedback on code quality, correctness, and suggest improvements.",
		},
		{
			ID:             "address_feedback",
			Label:          "Address feedback",
			Hint:           "Apply review comments",
			Icon:           "message",
			PromptTemplate: "Review the feedback on the pull request at {{url}}. Evaluate each comment critically — apply changes that improve the code, push back on suggestions that are unnecessary or harmful, and explain your reasoning. Push the changes when done.",
		},
		{
			ID:             "fix_ci",
			Label:          "Fix CI",
			Hint:           "Diagnose failing checks",
			Icon:           "tool",
			PromptTemplate: "Investigate and fix the CI failures and merge conflicts on the pull request at {{url}}. Run the failing checks locally, resolve any conflicts, diagnose issues, and push fixes.",
		},
	}
}

// DefaultIssueActionPresets returns the built-in Issue presets used when a
// workspace has no stored overrides.
func DefaultIssueActionPresets() []ActionPreset {
	return []ActionPreset{
		{
			ID:             "implement",
			Label:          "Implement",
			Hint:           "Build and open a PR",
			Icon:           "code",
			PromptTemplate: `Implement the changes described in the GitHub issue at {{url}} (title: "{{title}}"). Open a pull request when complete.`,
		},
		{
			ID:             "investigate",
			Label:          "Investigate",
			Hint:           "Find the root cause",
			Icon:           "search",
			PromptTemplate: `Investigate the GitHub issue at {{url}} (title: "{{title}}"). Identify root cause and summarize findings.`,
		},
		{
			ID:             "reproduce",
			Label:          "Reproduce",
			Hint:           "Document repro steps",
			Icon:           "bug",
			PromptTemplate: `Reproduce the bug described in the GitHub issue at {{url}} (title: "{{title}}"). Document the reproduction steps.`,
		},
	}
}
