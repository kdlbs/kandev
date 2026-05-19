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
type MRCommitInfo struct {
	SHA            string `json:"sha"`
	Message        string `json:"message"`
	AuthorUsername string `json:"author_username"`
	AuthorDate     string `json:"author_date"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	FilesChanged   int    `json:"files_changed"`
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
