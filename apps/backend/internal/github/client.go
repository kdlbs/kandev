package github

import (
	"context"
	"time"
)

// Client defines the interface for interacting with the GitHub API.
type Client interface {
	// IsAuthenticated checks if the client is authenticated with GitHub.
	IsAuthenticated(ctx context.Context) (bool, error)

	// GetAuthenticatedUser returns the username of the authenticated user.
	GetAuthenticatedUser(ctx context.Context) (string, error)

	// GetPR retrieves a single pull request by number.
	GetPR(ctx context.Context, owner, repo string, number int) (*PR, error)

	// FindPRByBranch finds an open PR for the given head branch.
	FindPRByBranch(ctx context.Context, owner, repo, branch string) (*PR, error)

	// ListAuthoredPRs lists open PRs authored by the authenticated user for a repo.
	ListAuthoredPRs(ctx context.Context, owner, repo string) ([]*PR, error)

	// ListReviewRequestedPRs lists open PRs where the user's review is requested.
	// filter is an optional GitHub search qualifier (e.g. "repo:owner/name" or "org:myorg").
	ListReviewRequestedPRs(ctx context.Context, filter string) ([]*PR, error)

	// ListUserOrgs returns the GitHub organizations the authenticated user belongs to.
	ListUserOrgs(ctx context.Context) ([]GitHubOrg, error)

	// SearchOrgRepos searches repositories in an organization, optionally filtered by a query string.
	SearchOrgRepos(ctx context.Context, org, query string, limit int) ([]GitHubRepo, error)

	// ListPRReviews lists reviews on a pull request.
	ListPRReviews(ctx context.Context, owner, repo string, number int) ([]PRReview, error)

	// ListPRComments lists review comments on a pull request.
	// If since is non-nil, only comments updated after that time are returned.
	ListPRComments(ctx context.Context, owner, repo string, number int, since *time.Time) ([]PRComment, error)

	// ListCheckRuns lists CI check runs for a given git ref (branch or SHA).
	ListCheckRuns(ctx context.Context, owner, repo, ref string) ([]CheckRun, error)

	// GetPRFeedback fetches aggregated feedback (reviews, comments, checks) for a PR.
	GetPRFeedback(ctx context.Context, owner, repo string, number int) (*PRFeedback, error)
}
