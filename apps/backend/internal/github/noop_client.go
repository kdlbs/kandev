package github

import (
	"context"
	"errors"
	"time"
)

// ErrNoClient is returned by NoopClient methods that cannot provide meaningful data.
var ErrNoClient = errors.New("github client not configured")

// NoopClient is a GitHub client that returns empty results for all operations.
// Used when GitHub integration is not configured or not needed.
type NoopClient struct{}

func (c *NoopClient) IsAuthenticated(context.Context) (bool, error) {
	return false, nil
}

func (c *NoopClient) GetAuthenticatedUser(context.Context) (string, error) {
	return "", nil
}

func (c *NoopClient) GetPR(context.Context, string, string, int) (*PR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) FindPRByBranch(context.Context, string, string, string) (*PR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListAuthoredPRs(context.Context, string, string) ([]*PR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListReviewRequestedPRs(context.Context, string, string, string) ([]*PR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListUserOrgs(context.Context) ([]GitHubOrg, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) SearchOrgRepos(context.Context, string, string, int) ([]GitHubRepo, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListPRReviews(context.Context, string, string, int) ([]PRReview, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListPRComments(context.Context, string, string, int, *time.Time) ([]PRComment, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListCheckRuns(context.Context, string, string, string) ([]CheckRun, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) GetPRFeedback(context.Context, string, string, int) (*PRFeedback, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListPRFiles(context.Context, string, string, int) ([]PRFile, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListPRCommits(context.Context, string, string, int) ([]PRCommitInfo, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) SubmitReview(context.Context, string, string, int, string, string) error {
	return ErrNoClient
}
