package gitlab

import "time"

// ProjectFilter identifies a GitLab project for review/issue watch filtering.
// Path is the namespace/path slug, e.g. "group/sub/project".
type ProjectFilter struct {
	Path string `json:"path"`
}

// ReviewScope controls which reviewer_username scope is used for the review
// watch. GitLab does not have team-level review requests, so the scope set is
// smaller than GitHub's, but we keep the constant set parallel for symmetry.
const (
	// ReviewScopeUser matches MRs where the user is explicitly the reviewer.
	ReviewScopeUser = "user"
	// ReviewScopeUserAndTeams is treated identically to ReviewScopeUser on
	// GitLab (no team review requests) — kept for cross-integration symmetry.
	ReviewScopeUserAndTeams = "user_and_teams"
)

// CleanupPolicy controls how a review or issue watch handles its auto-created
// tasks once the underlying MR / issue reaches a terminal state.
const (
	// CleanupPolicyAuto deletes the task once the MR/issue is merged or closed
	// UNLESS the user authored at least one message in the task (the agent's
	// auto-start prompt does not count).
	CleanupPolicyAuto = "auto"
	// CleanupPolicyAlways deletes the task on terminal state regardless of
	// user interaction.
	CleanupPolicyAlways = "always"
	// CleanupPolicyNever disables automatic cleanup.
	CleanupPolicyNever = "never"
)

// IsValidCleanupPolicy reports whether s is one of the recognized policies.
// Empty string is treated as valid so legacy rows default to "auto".
func IsValidCleanupPolicy(s string) bool {
	switch s {
	case "", CleanupPolicyAuto, CleanupPolicyAlways, CleanupPolicyNever:
		return true
	}
	return false
}

// NormalizeCleanupPolicy maps the empty string to CleanupPolicyAuto.
func NormalizeCleanupPolicy(s string) string {
	if s == "" {
		return CleanupPolicyAuto
	}
	return s
}

// MRWatch tracks active MR monitoring (session → MR). RepositoryID identifies
// which task repository the watched MR belongs to (multi-repo support; empty
// for single-repo tasks).
type MRWatch struct {
	ID                string     `json:"id" db:"id"`
	SessionID         string     `json:"session_id" db:"session_id"`
	TaskID            string     `json:"task_id" db:"task_id"`
	RepositoryID      string     `json:"repository_id,omitempty" db:"repository_id"`
	ProjectPath       string     `json:"project_path" db:"project_path"`
	MRIID             int        `json:"mr_iid" db:"mr_iid"`
	Branch            string     `json:"branch" db:"branch"`
	LastCheckedAt     *time.Time `json:"last_checked_at,omitempty" db:"last_checked_at"`
	LastNoteAt        *time.Time `json:"last_note_at,omitempty" db:"last_note_at"`
	LastPipelineState string     `json:"last_pipeline_state" db:"last_pipeline_state"`
	LastApprovalState string     `json:"last_approval_state" db:"last_approval_state"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

// ReviewWatch configures periodic polling for MRs needing the user's review.
// Projects holds the list of projects to monitor. An empty list means all
// projects the user has access to.
type ReviewWatch struct {
	ID                  string          `json:"id" db:"id"`
	WorkspaceID         string          `json:"workspace_id" db:"workspace_id"`
	WorkflowID          string          `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID      string          `json:"workflow_step_id" db:"workflow_step_id"`
	Projects            []ProjectFilter `json:"projects" db:"-"`
	ProjectsJSON        string          `json:"-" db:"projects"`
	AgentProfileID      string          `json:"agent_profile_id" db:"agent_profile_id"`
	ExecutorProfileID   string          `json:"executor_profile_id" db:"executor_profile_id"`
	Prompt              string          `json:"prompt" db:"prompt"`
	ReviewScope         string          `json:"review_scope" db:"review_scope"`
	CustomQuery         string          `json:"custom_query" db:"custom_query"`
	Enabled             bool            `json:"enabled" db:"enabled"`
	PollIntervalSeconds int             `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	CleanupPolicy       string          `json:"cleanup_policy" db:"cleanup_policy"`
	LastPolledAt        *time.Time      `json:"last_polled_at,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

// ReviewMRTask records which MRs have already had tasks created (dedup).
type ReviewMRTask struct {
	ID            string    `json:"id" db:"id"`
	ReviewWatchID string    `json:"review_watch_id" db:"review_watch_id"`
	ProjectPath   string    `json:"project_path" db:"project_path"`
	MRIID         int       `json:"mr_iid" db:"mr_iid"`
	MRURL         string    `json:"mr_url" db:"mr_url"`
	TaskID        string    `json:"task_id" db:"task_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// IssueWatch configures periodic polling for GitLab issues matching a query.
type IssueWatch struct {
	ID                  string          `json:"id" db:"id"`
	WorkspaceID         string          `json:"workspace_id" db:"workspace_id"`
	WorkflowID          string          `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID      string          `json:"workflow_step_id" db:"workflow_step_id"`
	Projects            []ProjectFilter `json:"projects" db:"-"`
	ProjectsJSON        string          `json:"-" db:"projects"`
	AgentProfileID      string          `json:"agent_profile_id" db:"agent_profile_id"`
	ExecutorProfileID   string          `json:"executor_profile_id" db:"executor_profile_id"`
	Prompt              string          `json:"prompt" db:"prompt"`
	Labels              []string        `json:"labels" db:"-"`
	LabelsJSON          string          `json:"-" db:"labels"`
	CustomQuery         string          `json:"custom_query" db:"custom_query"`
	Enabled             bool            `json:"enabled" db:"enabled"`
	PollIntervalSeconds int             `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	CleanupPolicy       string          `json:"cleanup_policy" db:"cleanup_policy"`
	LastPolledAt        *time.Time      `json:"last_polled_at,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

// IssueWatchTask records which issues have had tasks created (dedup).
type IssueWatchTask struct {
	ID           string    `json:"id" db:"id"`
	IssueWatchID string    `json:"issue_watch_id" db:"issue_watch_id"`
	ProjectPath  string    `json:"project_path" db:"project_path"`
	IssueIID     int       `json:"issue_iid" db:"issue_iid"`
	IssueURL     string    `json:"issue_url" db:"issue_url"`
	TaskID       string    `json:"task_id" db:"task_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// CreateReviewWatchRequest is the request body for creating a review watch.
type CreateReviewWatchRequest struct {
	WorkspaceID         string          `json:"workspace_id"`
	WorkflowID          string          `json:"workflow_id"`
	WorkflowStepID      string          `json:"workflow_step_id"`
	Projects            []ProjectFilter `json:"projects"`
	AgentProfileID      string          `json:"agent_profile_id"`
	ExecutorProfileID   string          `json:"executor_profile_id"`
	Prompt              string          `json:"prompt"`
	ReviewScope         string          `json:"review_scope"`
	CustomQuery         string          `json:"custom_query"`
	PollIntervalSeconds int             `json:"poll_interval_seconds"`
	CleanupPolicy       string          `json:"cleanup_policy"`
}

// UpdateReviewWatchRequest is the request body for updating a review watch.
type UpdateReviewWatchRequest struct {
	WorkflowID          *string          `json:"workflow_id,omitempty"`
	WorkflowStepID      *string          `json:"workflow_step_id,omitempty"`
	Projects            *[]ProjectFilter `json:"projects,omitempty"`
	AgentProfileID      *string          `json:"agent_profile_id,omitempty"`
	ExecutorProfileID   *string          `json:"executor_profile_id,omitempty"`
	Prompt              *string          `json:"prompt,omitempty"`
	ReviewScope         *string          `json:"review_scope,omitempty"`
	CustomQuery         *string          `json:"custom_query,omitempty"`
	Enabled             *bool            `json:"enabled,omitempty"`
	PollIntervalSeconds *int             `json:"poll_interval_seconds,omitempty"`
	CleanupPolicy       *string          `json:"cleanup_policy,omitempty"`
}

// CreateIssueWatchRequest is the request body for creating an issue watch.
type CreateIssueWatchRequest struct {
	WorkspaceID         string          `json:"workspace_id"`
	WorkflowID          string          `json:"workflow_id"`
	WorkflowStepID      string          `json:"workflow_step_id"`
	Projects            []ProjectFilter `json:"projects"`
	AgentProfileID      string          `json:"agent_profile_id"`
	ExecutorProfileID   string          `json:"executor_profile_id"`
	Prompt              string          `json:"prompt"`
	Labels              []string        `json:"labels"`
	CustomQuery         string          `json:"custom_query"`
	PollIntervalSeconds int             `json:"poll_interval_seconds"`
	CleanupPolicy       string          `json:"cleanup_policy"`
}

// UpdateIssueWatchRequest is the request body for updating an issue watch.
type UpdateIssueWatchRequest struct {
	WorkflowID          *string          `json:"workflow_id,omitempty"`
	WorkflowStepID      *string          `json:"workflow_step_id,omitempty"`
	Projects            *[]ProjectFilter `json:"projects,omitempty"`
	AgentProfileID      *string          `json:"agent_profile_id,omitempty"`
	ExecutorProfileID   *string          `json:"executor_profile_id,omitempty"`
	Prompt              *string          `json:"prompt,omitempty"`
	Labels              *[]string        `json:"labels,omitempty"`
	CustomQuery         *string          `json:"custom_query,omitempty"`
	Enabled             *bool            `json:"enabled,omitempty"`
	PollIntervalSeconds *int             `json:"poll_interval_seconds,omitempty"`
	CleanupPolicy       *string          `json:"cleanup_policy,omitempty"`
}

// MRFeedbackEvent is published when an MR has new feedback (new discussion,
// pipeline state change, approval state change). Used by the frontend to
// surface a toast / refresh the task.
type MRFeedbackEvent struct {
	SessionID        string `json:"session_id"`
	TaskID           string `json:"task_id"`
	ProjectPath      string `json:"project_path"`
	MRIID            int    `json:"mr_iid"`
	NewPipelineState string `json:"new_pipeline_state"`
	NewApprovalState string `json:"new_approval_state"`
}

// NewReviewMREvent is published when a new MR needing review is found by a
// review watch and a task has been auto-created.
type NewReviewMREvent struct {
	ReviewWatchID     string `json:"review_watch_id"`
	WorkspaceID       string `json:"workspace_id"`
	WorkflowID        string `json:"workflow_id"`
	WorkflowStepID    string `json:"workflow_step_id"`
	AgentProfileID    string `json:"agent_profile_id"`
	ExecutorProfileID string `json:"executor_profile_id"`
	Prompt            string `json:"prompt"`
	MR                *MR    `json:"mr"`
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

// --- Stats ---

// Stats aggregates GitLab activity counts surfaced on the /gitlab page.
type Stats struct {
	OpenMRs              int `json:"open_mrs"`
	MRsAwaitingMyReview  int `json:"mrs_awaiting_my_review"`
	OpenIssuesAssignedMe int `json:"open_issues_assigned_me"`
}

// --- Merge methods ---

// ProjectMergeMethods reports which merge methods a GitLab project allows.
// GitLab projects have a single `merge_method` setting; we expose it as a
// boolean set parallel to GitHub's allow_*_merge for the frontend.
type ProjectMergeMethods struct {
	Merge       bool `json:"merge"`        // GitLab `merge_method=merge`
	RebaseMerge bool `json:"rebase_merge"` // GitLab `merge_method=rebase_merge`
	FastForward bool `json:"fast_forward"` // GitLab `merge_method=ff`
	AllowSquash bool `json:"allow_squash"` // squash option on the merge request
}

// --- Action presets ---

// ActionPresetKind enumerates the two lists of quick-launch presets.
const (
	ActionPresetKindMR    = "mr"
	ActionPresetKindIssue = "issue"
)

// ActionPreset is a single configurable quick-task launcher entry.
// PromptTemplate supports `{{url}}` and `{{title}}` placeholders substituted
// client-side when the dialog is opened.
type ActionPreset struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Hint           string `json:"hint"`
	Icon           string `json:"icon"`
	PromptTemplate string `json:"prompt_template"`
}

// ActionPresets groups the MR and Issue preset lists for a workspace.
type ActionPresets struct {
	WorkspaceID string         `json:"workspace_id"`
	MR          []ActionPreset `json:"mr"`
	Issue       []ActionPreset `json:"issue"`
}

// UpdateActionPresetsRequest replaces one or both preset lists for a workspace.
// Nil fields leave the corresponding list unchanged.
type UpdateActionPresetsRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	MR          *[]ActionPreset `json:"mr,omitempty"`
	Issue       *[]ActionPreset `json:"issue,omitempty"`
}

// DefaultMRActionPresets returns the built-in MR presets used when a workspace
// has no stored overrides.
func DefaultMRActionPresets() []ActionPreset {
	return []ActionPreset{
		{
			ID:             "review",
			Label:          "Review",
			Hint:           "Read the diff, flag issues",
			Icon:           "eye",
			PromptTemplate: "Review the merge request at {{url}}. Provide feedback on code quality, correctness, and suggest improvements.",
		},
		{
			ID:             "address_feedback",
			Label:          "Address feedback",
			Hint:           "Apply review comments",
			Icon:           "message",
			PromptTemplate: "Review the feedback on the merge request at {{url}}. Evaluate each comment critically — apply changes that improve the code, push back on suggestions that are unnecessary or harmful, and explain your reasoning. Push the changes when done.",
		},
		{
			ID:             "fix_ci",
			Label:          "Fix pipeline",
			Hint:           "Diagnose failing jobs",
			Icon:           "tool",
			PromptTemplate: "Investigate and fix the failing pipeline jobs and merge conflicts on the merge request at {{url}}. Run the failing jobs locally, resolve any conflicts, diagnose issues, and push fixes.",
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
			Hint:           "Build and open an MR",
			Icon:           "code",
			PromptTemplate: `Implement the changes described in the GitLab issue at {{url}} (title: "{{title}}"). Open a merge request when complete.`,
		},
		{
			ID:             "investigate",
			Label:          "Investigate",
			Hint:           "Find the root cause",
			Icon:           "search",
			PromptTemplate: `Investigate the GitLab issue at {{url}} (title: "{{title}}"). Identify root cause and summarize findings.`,
		},
		{
			ID:             "reproduce",
			Label:          "Reproduce",
			Hint:           "Document repro steps",
			Icon:           "bug",
			PromptTemplate: `Reproduce the bug described in the GitLab issue at {{url}} (title: "{{title}}"). Document the reproduction steps.`,
		},
	}
}
