package gitlab

import (
	"context"
	"time"
)

// Client defines the interface for interacting with the GitLab API.
//
// Implementations: pat_client.go (REST v4 over HTTP), glab_client.go
// (shells out to the glab CLI), mock_client.go (in-memory, gated by
// KANDEV_MOCK_GITLAB=true), and noop_client.go (null-object fallback).
//
// projectPath everywhere is the namespace/path slug, e.g. "group/project"
// or "group/subgroup/project". The MR IID (per-project sequential ID) is
// the user-visible number.
type Client interface {
	// IsAuthenticated reports whether the client can talk to GitLab.
	IsAuthenticated(ctx context.Context) (bool, error)

	// GetAuthenticatedUser returns the username of the authenticated user.
	GetAuthenticatedUser(ctx context.Context) (string, error)

	// Host returns the GitLab host this client is configured against.
	Host() string

	// GetMR retrieves a single merge request by its per-project IID.
	GetMR(ctx context.Context, projectPath string, iid int) (*MR, error)

	// FindMRByBranch finds an open MR for the given source branch.
	FindMRByBranch(ctx context.Context, projectPath, branch string) (*MR, error)

	// ListAuthoredMRs lists open MRs authored by the authenticated user
	// for a project.
	ListAuthoredMRs(ctx context.Context, projectPath string) ([]*MR, error)

	// ListReviewRequestedMRs lists open MRs where the user is a reviewer.
	// filter is an optional additional GitLab API filter (e.g.
	// "project_id=123" or "milestone=v1"); customQuery, when non-empty,
	// replaces the entire generated query.
	ListReviewRequestedMRs(ctx context.Context, filter, customQuery string) ([]*MR, error)

	// ListUserGroups returns the GitLab groups the authenticated user
	// belongs to (analogous to GitHubOrg).
	ListUserGroups(ctx context.Context) ([]Group, error)

	// SearchGroupProjects searches projects in a group, optionally
	// filtered by a query string.
	SearchGroupProjects(ctx context.Context, group, query string, limit int) ([]Project, error)

	// ListMRApprovals lists approvals on a merge request.
	ListMRApprovals(ctx context.Context, projectPath string, iid int) ([]MRApproval, error)

	// ListMRDiscussions lists discussions (review threads) on an MR.
	// If since is non-nil, only discussions updated after that time are
	// returned.
	ListMRDiscussions(ctx context.Context, projectPath string, iid int, since *time.Time) ([]MRDiscussion, error)

	// CreateMRDiscussionNote posts a reply note in an existing discussion.
	CreateMRDiscussionNote(ctx context.Context, projectPath string, iid int, discussionID, body string) (*MRNote, error)

	// ResolveMRDiscussion marks a discussion as resolved.
	ResolveMRDiscussion(ctx context.Context, projectPath string, iid int, discussionID string) error

	// ListPipelines lists pipelines for a given git ref (branch or SHA).
	ListPipelines(ctx context.Context, projectPath, ref string) ([]Pipeline, error)

	// GetMRFeedback fetches aggregated feedback (approvals, discussions,
	// pipelines) for an MR.
	GetMRFeedback(ctx context.Context, projectPath string, iid int) (*MRFeedback, error)

	// GetMRStatus fetches lightweight MR state (used by the poller).
	GetMRStatus(ctx context.Context, projectPath string, iid int) (*MRStatus, error)

	// ListMRFiles lists files changed in a merge request.
	ListMRFiles(ctx context.Context, projectPath string, iid int) ([]MRFile, error)

	// ListMRCommits lists commits in a merge request.
	ListMRCommits(ctx context.Context, projectPath string, iid int) ([]MRCommitInfo, error)

	// SubmitMRApproval approves an MR. To revoke an approval, call
	// SubmitMRUnapproval.
	SubmitMRApproval(ctx context.Context, projectPath string, iid int) error

	// SubmitMRUnapproval revokes the authenticated user's approval of an MR.
	SubmitMRUnapproval(ctx context.Context, projectPath string, iid int) error

	// CreateMR opens a new merge request. Used by the agent `pr` skill.
	CreateMR(ctx context.Context, projectPath, sourceBranch, targetBranch, title, description string, draft bool) (*MR, error)

	// ListProjectBranches lists branches for a project.
	ListProjectBranches(ctx context.Context, projectPath string) ([]RepoBranch, error)

	// ListIssues searches for open issues. filter is an optional
	// additional API filter; customQuery, when non-empty, replaces the
	// entire generated query.
	ListIssues(ctx context.Context, filter, customQuery string) ([]*Issue, error)

	// SearchMRs searches for MRs matching the given query.
	SearchMRs(ctx context.Context, filter, customQuery string) ([]*MR, error)

	// SearchMRsPaged is the paginated variant of SearchMRs. page is
	// 1-indexed; perPage is clamped to GitLab's 1..100 range.
	SearchMRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*MRSearchPage, error)

	// ListIssuesPaged is the paginated variant of ListIssues.
	ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error)

	// GetIssueState returns the state of a single issue ("opened" or "closed").
	GetIssueState(ctx context.Context, projectPath string, iid int) (string, error)
}
