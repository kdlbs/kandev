// Package gitlab provides GitLab integration for Kandev: merge request
// monitoring, review queue management, pipeline status tracking, and
// discussion (review comment) interaction. Mirrors the surface of the
// internal/github package, adapted to GitLab nouns and the REST v4 API.
package gitlab

import "time"

// MR represents a GitLab Merge Request.
//
// IID is GitLab's per-project sequential ID (the number shown in the UI).
// ID is the global GitLab ID — required for some endpoints. The frontend
// keys on (ProjectPath, IID).
type MR struct {
	ID               int64        `json:"id"`
	IID              int          `json:"iid"`
	ProjectID        int64        `json:"project_id"`
	Title            string       `json:"title"`
	URL              string       `json:"url"`
	WebURL           string       `json:"web_url"`
	State            string       `json:"state"` // open, closed, merged, locked
	HeadBranch       string       `json:"head_branch"`
	HeadSHA          string       `json:"head_sha"`
	BaseBranch       string       `json:"base_branch"`
	AuthorUsername   string       `json:"author_username"`
	ProjectNamespace string       `json:"project_namespace"`
	ProjectPath      string       `json:"project_path"`
	Body             string       `json:"body"`
	Draft            bool         `json:"draft"`
	MergeStatus      string       `json:"merge_status"` // can_be_merged, cannot_be_merged, unchecked, ...
	HasConflicts     bool         `json:"has_conflicts"`
	Additions        int          `json:"additions"`
	Deletions        int          `json:"deletions"`
	Reviewers        []MRReviewer `json:"reviewers"`
	Assignees        []MRReviewer `json:"assignees"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	MergedAt         *time.Time   `json:"merged_at,omitempty"`
	ClosedAt         *time.Time   `json:"closed_at,omitempty"`
}

// MRReviewer represents a reviewer or assignee on an MR.
// GitLab does not have team-level review requests, so Type is always "user".
type MRReviewer struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Type     string `json:"type"`
}

// MRApproval represents a single approval on a merge request.
// GitLab approvals are simpler than GitHub reviews — there is no
// COMMENTED / DISMISSED state, only approved/not-approved.
type MRApproval struct {
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	CreatedAt time.Time `json:"created_at"`
}

// MRDiscussion represents a discussion thread on a merge request.
// Discussions are GitLab's review-comment unit: a top-level note, optionally
// anchored to a file/line, and zero or more reply notes. The Resolved flag
// is per-discussion (not per-note).
type MRDiscussion struct {
	ID         string    `json:"id"`
	Resolvable bool      `json:"resolvable"`
	Resolved   bool      `json:"resolved"`
	Notes      []MRNote  `json:"notes"`
	Path       string    `json:"path,omitempty"`
	Line       int       `json:"line,omitempty"`
	OldLine    int       `json:"old_line,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// MRNote is a single message inside a discussion thread.
type MRNote struct {
	ID           int64     `json:"id"`
	Author       string    `json:"author"`
	AuthorAvatar string    `json:"author_avatar"`
	AuthorIsBot  bool      `json:"author_is_bot"`
	Body         string    `json:"body"`
	Type         string    `json:"type"` // DiffNote, DiscussionNote, or empty for regular notes
	System       bool      `json:"system"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Pipeline represents a CI pipeline run associated with an MR.
// GitLab pipelines are richer than GitHub check runs — they have stages
// and jobs — but the UI surface only needs the rolled-up status.
type Pipeline struct {
	ID          int64      `json:"id"`
	IID         int        `json:"iid"`
	Status      string     `json:"status"` // running, pending, success, failed, canceled, skipped, manual, ...
	Source      string     `json:"source"` // push, merge_request_event, schedule, ...
	Ref         string     `json:"ref"`
	SHA         string     `json:"sha"`
	WebURL      string     `json:"web_url"`
	JobsTotal   int        `json:"jobs_total"`
	JobsPassing int        `json:"jobs_passing"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// MRFeedback aggregates all feedback for an MR (fetched live from GitLab).
type MRFeedback struct {
	MR          *MR            `json:"mr"`
	Approvals   []MRApproval   `json:"approvals"`
	Discussions []MRDiscussion `json:"discussions"`
	Pipelines   []Pipeline     `json:"pipelines"`
	HasIssues   bool           `json:"has_issues"`
}

// MRStatus contains lightweight MR state used by the background poller.
// Unlike MRFeedback it skips discussions to reduce API calls.
type MRStatus struct {
	MR                  *MR    `json:"mr"`
	ApprovalState       string `json:"approval_state"` // "approved", "changes_requested", "pending", ""
	PipelineState       string `json:"pipeline_state"` // "success", "failure", "pending", ""
	MergeStatus         string `json:"merge_status"`   // can_be_merged, cannot_be_merged, unchecked
	ApprovalCount       int    `json:"approval_count"`
	RequiredApprovals   int    `json:"required_approvals"`
	PipelineJobsTotal   int    `json:"pipeline_jobs_total"`
	PipelineJobsPassing int    `json:"pipeline_jobs_passing"`
}

// MRSearchPage is a paginated slice of MR search results, with the total
// count from the GitLab API X-Total-Pages / X-Total headers.
type MRSearchPage struct {
	MRs        []*MR `json:"mrs"`
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

// MRWatch tracks active MR monitoring (session → MR). RepositoryID identifies
// which task repository the watched MR belongs to (multi-repo support).
type MRWatch struct {
	ID                 string     `json:"id" db:"id"`
	SessionID          string     `json:"session_id" db:"session_id"`
	TaskID             string     `json:"task_id" db:"task_id"`
	RepositoryID       string     `json:"repository_id,omitempty" db:"repository_id"`
	ProjectPath        string     `json:"project_path" db:"project_path"`
	MRIID              int        `json:"mr_iid" db:"mr_iid"`
	Branch             string     `json:"branch" db:"branch"`
	LastCheckedAt      *time.Time `json:"last_checked_at,omitempty" db:"last_checked_at"`
	LastNoteAt         *time.Time `json:"last_note_at,omitempty" db:"last_note_at"`
	LastPipelineStatus string     `json:"last_pipeline_status" db:"last_pipeline_status"`
	LastApprovalState  string     `json:"last_approval_state" db:"last_approval_state"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskMR associates an MR with a task. RepositoryID identifies which task
// repository this MR belongs to.
type TaskMR struct {
	ID                string     `json:"id" db:"id"`
	TaskID            string     `json:"task_id" db:"task_id"`
	RepositoryID      string     `json:"repository_id,omitempty" db:"repository_id"`
	ProjectPath       string     `json:"project_path" db:"project_path"`
	MRIID             int        `json:"mr_iid" db:"mr_iid"`
	MRURL             string     `json:"mr_url" db:"mr_url"`
	MRTitle           string     `json:"mr_title" db:"mr_title"`
	HeadBranch        string     `json:"head_branch" db:"head_branch"`
	BaseBranch        string     `json:"base_branch" db:"base_branch"`
	AuthorUsername    string     `json:"author_username" db:"author_username"`
	State             string     `json:"state" db:"state"`
	ApprovalState     string     `json:"approval_state" db:"approval_state"`
	PipelineState     string     `json:"pipeline_state" db:"pipeline_state"`
	MergeStatus       string     `json:"merge_status" db:"merge_status"`
	ApprovalCount     int        `json:"approval_count" db:"approval_count"`
	RequiredApprovals int        `json:"required_approvals" db:"required_approvals"`
	NoteCount         int        `json:"note_count" db:"note_count"`
	Additions         int        `json:"additions" db:"additions"`
	Deletions         int        `json:"deletions" db:"deletions"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	MergedAt          *time.Time `json:"merged_at,omitempty" db:"merged_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty" db:"closed_at"`
	LastSyncedAt      *time.Time `json:"last_synced_at,omitempty" db:"last_synced_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

// ProjectFilter identifies a GitLab project for review-watch / issue-watch
// filtering. Path is the namespace/path slug (e.g. "group/subgroup/project").
type ProjectFilter struct {
	Path string `json:"path"`
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
	CustomQuery         string          `json:"custom_query" db:"custom_query"`
	Enabled             bool            `json:"enabled" db:"enabled"`
	PollIntervalSeconds int             `json:"poll_interval_seconds" db:"poll_interval_seconds"`
	LastPolledAt        *time.Time      `json:"last_polled_at,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

// ReviewMRTask records which MRs already had tasks created (deduplication).
type ReviewMRTask struct {
	ID            string    `json:"id" db:"id"`
	ReviewWatchID string    `json:"review_watch_id" db:"review_watch_id"`
	ProjectPath   string    `json:"project_path" db:"project_path"`
	MRIID         int       `json:"mr_iid" db:"mr_iid"`
	MRURL         string    `json:"mr_url" db:"mr_url"`
	TaskID        string    `json:"task_id" db:"task_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// Project represents a GitLab project (lightweight, for autocomplete).
type Project struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	Namespace         string `json:"namespace"`
	Path              string `json:"path"`
	Name              string `json:"name"`
	Visibility        string `json:"visibility"` // private, internal, public
}

// Group represents a GitLab group the authenticated user belongs to.
// Analogous to GitHubOrg.
type Group struct {
	ID        int64  `json:"id"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// RepoBranch represents a branch in a GitLab project.
type RepoBranch struct {
	Name string `json:"name"`
}

// Status represents GitLab connection status surfaced to the frontend.
type Status struct {
	Authenticated   bool             `json:"authenticated"`
	Username        string           `json:"username"`
	AuthMethod      string           `json:"auth_method"` // "glab_cli", "pat", "none"
	Host            string           `json:"host"`
	TokenConfigured bool             `json:"token_configured"`
	TokenSecretID   string           `json:"token_secret_id,omitempty"`
	GLabVersion     string           `json:"glab_version,omitempty"`
	GLabOutdated    bool             `json:"glab_outdated,omitempty"`
	RequiredScopes  []string         `json:"required_scopes"`
	Diagnostics     *AuthDiagnostics `json:"diagnostics,omitempty"`
}

// ConfigureTokenRequest is the request body for configuring a GitLab token.
type ConfigureTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// ConfigureHostRequest is the request body for setting the GitLab host URL.
type ConfigureHostRequest struct {
	Host string `json:"host" binding:"required"`
}

// AuthDiagnostics captures the output of `glab auth status` (or REST probe)
// for troubleshooting.
type AuthDiagnostics struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
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
	CustomQuery         string          `json:"custom_query"`
	PollIntervalSeconds int             `json:"poll_interval_seconds"`
}

// UpdateReviewWatchRequest is the request body for updating a review watch.
type UpdateReviewWatchRequest struct {
	WorkflowID          *string          `json:"workflow_id,omitempty"`
	WorkflowStepID      *string          `json:"workflow_step_id,omitempty"`
	Projects            *[]ProjectFilter `json:"projects,omitempty"`
	AgentProfileID      *string          `json:"agent_profile_id,omitempty"`
	ExecutorProfileID   *string          `json:"executor_profile_id,omitempty"`
	Prompt              *string          `json:"prompt,omitempty"`
	CustomQuery         *string          `json:"custom_query,omitempty"`
	Enabled             *bool            `json:"enabled,omitempty"`
	PollIntervalSeconds *int             `json:"poll_interval_seconds,omitempty"`
}

// MRFeedbackEvent is published when an MR has new feedback.
type MRFeedbackEvent struct {
	SessionID         string `json:"session_id"`
	TaskID            string `json:"task_id"`
	ProjectPath       string `json:"project_path"`
	MRIID             int    `json:"mr_iid"`
	NewPipelineStatus string `json:"new_pipeline_status"`
	NewApprovalState  string `json:"new_approval_state"`
}

// NewReviewMREvent is published when a new MR needing review is found.
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

// MRStatsRequest defines filters for MR stats queries.
type MRStatsRequest struct {
	WorkspaceID string     `json:"workspace_id"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
}

// MRStats holds aggregated MR analytics.
type MRStats struct {
	TotalMRsCreated     int          `json:"total_mrs_created"`
	TotalMRsReviewed    int          `json:"total_mrs_reviewed"`
	TotalNotes          int          `json:"total_notes"`
	PipelinePassRate    float64      `json:"pipeline_pass_rate"`
	ApprovalRate        float64      `json:"approval_rate"`
	AvgTimeToMergeHours float64      `json:"avg_time_to_merge_hours"`
	MRsByDay            []DailyCount `json:"mrs_by_day"`
}

// MRFile represents a file changed in a merge request.
type MRFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"` // added, deleted, modified, renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
	OldPath   string `json:"old_path,omitempty"`
}

// MRCommitInfo represents a commit in a merge request.
type MRCommitInfo struct {
	SHA            string `json:"sha"`
	Message        string `json:"message"`
	AuthorUsername string `json:"author_username"`
	AuthorDate     string `json:"author_date"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	FilesChanged   int    `json:"files_changed"`
}

// DailyCount holds a date and count for chart data.
type DailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// --- Issue Watch models ---

// Issue represents a GitLab Issue.
type Issue struct {
	ID               int64      `json:"id"`
	IID              int        `json:"iid"`
	ProjectID        int64      `json:"project_id"`
	Title            string     `json:"title"`
	Body             string     `json:"body"`
	URL              string     `json:"url"`
	WebURL           string     `json:"web_url"`
	State            string     `json:"state"` // opened, closed
	AuthorUsername   string     `json:"author_username"`
	ProjectNamespace string     `json:"project_namespace"`
	ProjectPath      string     `json:"project_path"`
	Labels           []string   `json:"labels"`
	Assignees        []string   `json:"assignees"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
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
	LastPolledAt        *time.Time      `json:"last_polled_at,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

// IssueWatchTask records which issues already had tasks created (dedup).
type IssueWatchTask struct {
	ID           string    `json:"id" db:"id"`
	IssueWatchID string    `json:"issue_watch_id" db:"issue_watch_id"`
	ProjectPath  string    `json:"project_path" db:"project_path"`
	IssueIID     int       `json:"issue_iid" db:"issue_iid"`
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
}

// --- Action presets (quick-launch prompts on the /gitlab page) ---

// ActionPresetKind enumerates the two lists of quick-launch presets.
const (
	ActionPresetKindMR    = "mr"
	ActionPresetKindIssue = "issue"
)

// ActionPreset is a single configurable quick-task launcher entry.
// PromptTemplate supports `{{url}}` and `{{title}}` placeholders.
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

// UpdateActionPresetsRequest replaces one or both preset lists.
type UpdateActionPresetsRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	MR          *[]ActionPreset `json:"mr,omitempty"`
	Issue       *[]ActionPreset `json:"issue,omitempty"`
}

// DefaultMRActionPresets returns the built-in MR presets used when a
// workspace has no stored overrides. Mirrors the GitHub PR defaults.
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
			PromptTemplate: "Review the discussions on the merge request at {{url}}. Evaluate each comment critically — apply changes that improve the code, push back on suggestions that are unnecessary or harmful, and explain your reasoning. Resolve discussions you have addressed and push the changes when done.",
		},
		{
			ID:             "fix_pipeline",
			Label:          "Fix pipeline",
			Hint:           "Diagnose failing jobs",
			Icon:           "tool",
			PromptTemplate: "Investigate and fix the pipeline failures and merge conflicts on the merge request at {{url}}. Run the failing jobs locally where possible, resolve any conflicts, diagnose issues, and push fixes.",
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
