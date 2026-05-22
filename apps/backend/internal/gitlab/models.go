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
// count from the GitLab API X-Total header.
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
	// ConnectionError carries a transport-layer failure from the most
	// recent IsAuthenticated probe (network down, 5xx, parse failure).
	// 401/403 is NOT a connection error — that just means Authenticated
	// is false. Empty when the probe succeeded or the host is unconfigured.
	// Lets the frontend distinguish "not connected" from "GitLab is
	// temporarily unreachable" so users don't delete a valid token during
	// a transient outage.
	ConnectionError string `json:"connection_error,omitempty"`
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
//
// AuthorName is the commit's `author_name` (display name like "Jane Smith"),
// not a GitLab login handle — GitLab's MR-commits endpoint doesn't expose a
// login. Per-commit Additions/Deletions/FilesChanged are also not on the
// MR-commits endpoint; getting them would require a per-commit fetch
// against /projects/:id/repository/commits/:sha?stats=true, which is N+1
// and out of scope for v1. The fields were filled with 0 in earlier
// revisions and removed here to stop callers consuming bogus data.
type MRCommitInfo struct {
	SHA        string `json:"sha"`
	Message    string `json:"message"`
	AuthorName string `json:"author_name"`
	AuthorDate string `json:"author_date"`
}

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

// TaskMR associates a kandev task with a GitLab merge request, parallel to
// github.TaskPR. RepositoryID may be empty for single-repo tasks; multi-repo
// task launches set it so each repo's MR is distinguishable on the same
// (task, iid) key. ProjectPath holds the GitLab path-with-namespace
// ("group/project") used as the API :id.
//
// GitLab-shaped fields differ from GitHub:
//   - State is "open" | "closed" | "merged" | "locked" (normalised from
//     GitLab's "opened" wording).
//   - ApprovalState is "" | "approved" | "pending" — derived from approvals
//     received vs RequiredApprovals.
//   - PipelineState mirrors GitHub's ChecksState ("success", "failure",
//     "pending", "") — GitLab pipelines are the analogue of GitHub checks.
//   - MergeStatus carries GitLab's own merge_status string verbatim
//     (can_be_merged, cannot_be_merged, unchecked, …) for debugging.
type TaskMR struct {
	ID                string     `json:"id" db:"id"`
	TaskID            string     `json:"task_id" db:"task_id"`
	RepositoryID      string     `json:"repository_id,omitempty" db:"repository_id"`
	Host              string     `json:"host" db:"host"`                 // gitlab base URL the MR lives on
	ProjectPath       string     `json:"project_path" db:"project_path"` // namespace/path
	MRIID             int        `json:"mr_iid" db:"mr_iid"`             // GitLab per-project sequential id
	MRURL             string     `json:"mr_url" db:"mr_url"`
	MRTitle           string     `json:"mr_title" db:"mr_title"`
	HeadBranch        string     `json:"head_branch" db:"head_branch"`
	BaseBranch        string     `json:"base_branch" db:"base_branch"`
	AuthorUsername    string     `json:"author_username" db:"author_username"`
	State             string     `json:"state" db:"state"`                   // open, closed, merged, locked
	ApprovalState     string     `json:"approval_state" db:"approval_state"` // approved, pending, ""
	PipelineState     string     `json:"pipeline_state" db:"pipeline_state"` // success, failure, pending, ""
	MergeStatus       string     `json:"merge_status" db:"merge_status"`
	Draft             bool       `json:"draft" db:"draft"`
	ApprovalCount     int        `json:"approval_count" db:"approval_count"`
	RequiredApprovals int        `json:"required_approvals" db:"required_approvals"`
	PipelineJobsTotal int        `json:"pipeline_jobs_total" db:"pipeline_jobs_total"`
	PipelineJobsPass  int        `json:"pipeline_jobs_pass" db:"pipeline_jobs_pass"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	MergedAt          *time.Time `json:"merged_at,omitempty" db:"merged_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty" db:"closed_at"`
	LastSyncedAt      *time.Time `json:"last_synced_at,omitempty" db:"last_synced_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

// TaskMRsResponse is the shape returned by GET /workspaces/:id/task-mrs.
// Keyed by task ID; each task may have one entry per linked repository.
type TaskMRsResponse struct {
	TaskMRs map[string][]*TaskMR `json:"task_mrs"`
}
