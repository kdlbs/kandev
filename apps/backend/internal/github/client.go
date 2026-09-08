package github

import (
	"context"
	"fmt"
	"time"
)

// RateLimitFetcher exposes the optional initial rate-limit probe supported by
// authenticated GitHub clients. It is deliberately separate from Client so
// in-memory and specialized clients do not need to implement a settings-only
// operation.
type RateLimitFetcher interface {
	FetchRateLimit(ctx context.Context) error
}

type MergeOutcome string

const (
	MergeOutcomeMerged MergeOutcome = "merged"
	MergeOutcomeQueued MergeOutcome = "queued"
	mergeStatusFailed               = "failed"
	mergePollInterval               = time.Second
)

// MergePRRequest defines the optional merge method and the pull-request head
// that GitHub must still expose when it accepts the merge.
type MergePRRequest struct {
	MergeMethod     string
	ExpectedHeadSHA string
}

type mergeAsyncResponse struct {
	Status  string                    `json:"status"`
	Details mergeAsyncResponseDetails `json:"details"`
}

type mergeAsyncResponseDetails struct {
	UUID            string `json:"uuid"`
	Message         string `json:"message"`
	ExpectedHeadSHA string `json:"expected_head_sha"`
}

func waitForMergePoll(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func newMergeFailureError(details mergeAsyncResponseDetails) error {
	if details.ExpectedHeadSHA != "" {
		return fmt.Errorf("GitHub rejected merge for expected head %s: %s", details.ExpectedHeadSHA, details.Message)
	}
	return fmt.Errorf("GitHub rejected merge: %s", details.Message)
}

func normalizeMergeOutcome(status string) (MergeOutcome, error) {
	switch status {
	case string(MergeOutcomeMerged):
		return MergeOutcomeMerged, nil
	case "enqueued", "queued":
		return MergeOutcomeQueued, nil
	case mergeStatusFailed:
		return "", fmt.Errorf("GitHub rejected merge: %s", status)
	default:
		return "", fmt.Errorf("unexpected GitHub merge status %q", status)
	}
}

// Client defines the interface for interacting with the GitHub API.
type Client interface {
	// IsAuthenticated checks if the client is authenticated with GitHub.
	IsAuthenticated(ctx context.Context) (bool, error)

	// GetAuthenticatedUser returns the username of the authenticated user.
	GetAuthenticatedUser(ctx context.Context) (string, error)

	// GetPR retrieves a single pull request by number.
	GetPR(ctx context.Context, owner, repo string, number int) (*PR, error)

	// GetIssue retrieves a single GitHub issue by number.
	GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error)

	// FindPRByBranch finds an open PR for the given head branch.
	FindPRByBranch(ctx context.Context, owner, repo, branch string) (*PR, error)

	// ListAuthoredPRs lists open PRs authored by the authenticated user for a repo.
	ListAuthoredPRs(ctx context.Context, owner, repo string) ([]*PR, error)

	// ListReviewRequestedPRs lists open PRs where the user's review is requested.
	// scope controls the search qualifier: ReviewScopeUser uses user-review-requested:@me,
	// ReviewScopeUserAndTeams (or empty) uses review-requested:@me.
	// filter is an optional additional search qualifier (e.g. "repo:owner/name" or "org:myorg").
	// customQuery, when non-empty, replaces the entire generated query.
	ListReviewRequestedPRs(ctx context.Context, scope, filter, customQuery string) ([]*PR, error)

	// ListUserOrgs returns the GitHub organizations the authenticated user belongs to.
	ListUserOrgs(ctx context.Context) ([]GitHubOrg, error)

	// SearchOrgRepos searches repositories in an organization, optionally filtered by a query string.
	SearchOrgRepos(ctx context.Context, org, query string, limit int) ([]GitHubRepo, error)

	// ListUserRepos lists repositories the authenticated user has access to,
	// optionally filtered by a query string. limit is a per-page upper bound
	// (clamped to GitHub's 100 max by implementations).
	ListUserRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error)

	// ListAccessibleRepos lists every repo the authenticated user can access —
	// their own repos plus collaborator and org-member repos — in a single
	// GET /user/repos call on the core REST quota (no per-org search fan-out).
	// query is applied as a case-insensitive substring filter on full_name
	// after fetching; limit bounds the page size (clamped to GitHub's 100 max).
	ListAccessibleRepos(ctx context.Context, query string, limit int) ([]GitHubRepo, error)

	// ListPRReviews lists reviews on a pull request.
	ListPRReviews(ctx context.Context, owner, repo string, number int) ([]PRReview, error)

	// ListPRComments lists review comments on a pull request.
	// If since is non-nil, only comments updated after that time are returned.
	ListPRComments(ctx context.Context, owner, repo string, number int, since *time.Time) ([]PRComment, error)

	// ListCheckRuns lists CI check runs for a given git ref (branch or SHA).
	ListCheckRuns(ctx context.Context, owner, repo, ref string) ([]CheckRun, error)

	// GetPRFeedback fetches aggregated feedback (reviews, comments, checks) for a PR.
	GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error)

	// GetPRStatus fetches lightweight PR status (state, review summary, checks summary).
	// Unlike GetPRFeedback, it skips comments for efficiency.
	GetPRStatus(ctx context.Context, owner, repo string, number int) (*PRStatus, error)

	// ListPRFiles lists files changed in a pull request.
	ListPRFiles(ctx context.Context, owner, repo string, number int) ([]PRFile, error)

	// ListPRCommits lists commits in a pull request.
	ListPRCommits(ctx context.Context, owner, repo string, number int) ([]PRCommitInfo, error)

	// GetPRCommitDetail retrieves one commit and all of its changed files.
	GetPRCommitDetail(ctx context.Context, owner, repo, sha string) (PRCommitDetail, error)

	// SubmitReview submits a review on a pull request.
	// event is one of "APPROVE", "COMMENT", "REQUEST_CHANGES".
	SubmitReview(ctx context.Context, owner, repo string, number int, event, body string) error

	// RequestReviewers requests reviews from GitHub users on a pull request.
	RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error

	// MergePR merges a pull request immediately or queues it according to GitHub policy.
	MergePR(ctx context.Context, owner, repo string, number int, request MergePRRequest) (MergeOutcome, error)

	// ListRepoBranches lists branches for a repository.
	ListRepoBranches(ctx context.Context, owner, repo string) ([]RepoBranch, error)

	// GetRepoMergeMethods reports which merge methods a repository allows.
	GetRepoMergeMethods(ctx context.Context, owner, repo string) (RepoMergeMethods, error)

	// ListIssues searches for open issues (not PRs) matching the given query.
	// filter is an optional additional search qualifier (e.g. "repo:owner/name" or "label:bug").
	// customQuery, when non-empty, replaces the entire generated query.
	ListIssues(ctx context.Context, filter, customQuery string) ([]*Issue, error)

	// SearchPRs searches for PRs matching the given query.
	// filter is an optional additional search qualifier (e.g. "author:@me" or "repo:owner/name").
	// customQuery, when non-empty, replaces the entire generated query.
	SearchPRs(ctx context.Context, filter, customQuery string) ([]*PR, error)

	// SearchPRsPaged is the paginated variant of SearchPRs. page is 1-indexed;
	// perPage is clamped to GitHub's 1..100 range by the implementation.
	SearchPRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*PRSearchPage, error)

	// ListIssuesPaged is the paginated variant of ListIssues.
	ListIssuesPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error)

	// GetIssueState returns the state of a single issue ("open" or "closed").
	GetIssueState(ctx context.Context, owner, repo string, number int) (string, error)

	// CreateGist creates a new gist on the authenticated user's account.
	// Set Public=false to create a secret (unlisted) gist.
	CreateGist(ctx context.Context, in CreateGistInput) (*GistResponse, error)

	// DeleteGist deletes a gist by ID. A 404 is wrapped in *GitHubAPIError
	// so callers can distinguish "already gone" from transport failures.
	DeleteGist(ctx context.Context, gistID string) error

	// ListRepoDirectory lists the entries of a directory in a repository at the
	// given ref (branch, tag, or SHA). dir may be "" or "." for the repo root.
	// A missing directory returns a *GitHubAPIError with status 404.
	ListRepoDirectory(ctx context.Context, owner, repo, dir, ref string) ([]RepoContentEntry, error)

	// GetRepoFileContent fetches the raw decoded content of a single file in a
	// repository at the given ref. A missing file returns a *GitHubAPIError with
	// status 404.
	GetRepoFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
}
