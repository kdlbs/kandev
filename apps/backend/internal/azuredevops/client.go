package azuredevops

import (
	"context"
	"errors"
)

// ErrNotConfigured is returned when a workspace has no complete Azure DevOps
// connection.
var ErrNotConfigured = errors.New("azure devops: workspace not configured")

// Client is the Azure DevOps read surface consumed by the integration service.
type Client interface {
	TestAuth(ctx context.Context) (*TestConnectionResult, error)
	ListProjects(ctx context.Context) ([]Project, error)
	ListRepositories(ctx context.Context, projectID string) ([]Repository, error)
	QueryWIQL(ctx context.Context, projectID, wiql string, top int) (*WorkItemSearchResult, error)
	GetWorkItem(ctx context.Context, projectID string, id int) (*WorkItem, error)
	ListPullRequests(ctx context.Context, filter PullRequestFilter) (*PullRequestPage, error)
	GetPullRequest(ctx context.Context, projectID, repositoryID string, id int) (*PullRequest, error)
	ListReviewers(ctx context.Context, projectID, repositoryID string, id int) ([]Reviewer, error)
	ListThreads(ctx context.Context, projectID, repositoryID string, id int) ([]Thread, error)
	ListLinkedWorkItems(ctx context.Context, projectID, repositoryID string, id int) ([]WorkItemRef, error)
	ListPolicyEvaluations(ctx context.Context, projectID string, id int) ([]PolicyEvaluation, error)
}

// ClientFactory constructs a client from one workspace's configuration and
// PAT. Tests inject a deterministic fake at this boundary.
type ClientFactory func(cfg *Config, pat string) Client

// DefaultClientFactory constructs the direct REST implementation. No Azure or
// GitHub CLI is consulted.
func DefaultClientFactory(cfg *Config, pat string) Client {
	if cfg == nil {
		return &invalidClient{err: errors.New("azure devops: config is required")}
	}
	return NewRESTClient(cfg.OrganizationURL, pat, nil)
}

type invalidClient struct{ err error }

func (c *invalidClient) TestAuth(context.Context) (*TestConnectionResult, error) { return nil, c.err }
func (c *invalidClient) ListProjects(context.Context) ([]Project, error)         { return nil, c.err }
func (c *invalidClient) ListRepositories(context.Context, string) ([]Repository, error) {
	return nil, c.err
}
func (c *invalidClient) QueryWIQL(context.Context, string, string, int) (*WorkItemSearchResult, error) {
	return nil, c.err
}
func (c *invalidClient) GetWorkItem(context.Context, string, int) (*WorkItem, error) {
	return nil, c.err
}
func (c *invalidClient) ListPullRequests(context.Context, PullRequestFilter) (*PullRequestPage, error) {
	return nil, c.err
}
func (c *invalidClient) GetPullRequest(context.Context, string, string, int) (*PullRequest, error) {
	return nil, c.err
}
func (c *invalidClient) ListReviewers(context.Context, string, string, int) ([]Reviewer, error) {
	return nil, c.err
}
func (c *invalidClient) ListThreads(context.Context, string, string, int) ([]Thread, error) {
	return nil, c.err
}
func (c *invalidClient) ListLinkedWorkItems(context.Context, string, string, int) ([]WorkItemRef, error) {
	return nil, c.err
}
func (c *invalidClient) ListPolicyEvaluations(context.Context, string, int) ([]PolicyEvaluation, error) {
	return nil, c.err
}
