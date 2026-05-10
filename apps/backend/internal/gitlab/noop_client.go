package gitlab

import (
	"context"
	"errors"
	"time"
)

// ErrNoClient is returned by NoopClient methods that cannot provide
// meaningful data.
var ErrNoClient = errors.New("gitlab client not configured")

// NoopClient is a GitLab client that returns empty results for all
// operations. Used when GitLab integration is not configured.
type NoopClient struct {
	host string
}

// NewNoopClient builds a NoopClient that reports the given host (purely
// informational; no requests are issued).
func NewNoopClient(host string) *NoopClient {
	if host == "" {
		host = DefaultHost
	}
	return &NoopClient{host: host}
}

func (c *NoopClient) IsAuthenticated(context.Context) (bool, error) {
	return false, nil
}

func (c *NoopClient) GetAuthenticatedUser(context.Context) (string, error) {
	return "", nil
}

func (c *NoopClient) Host() string { return c.host }

func (c *NoopClient) GetMR(context.Context, string, int) (*MR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) FindMRByBranch(context.Context, string, string) (*MR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListAuthoredMRs(context.Context, string) ([]*MR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListReviewRequestedMRs(context.Context, string, string) ([]*MR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListUserGroups(context.Context) ([]Group, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) SearchGroupProjects(context.Context, string, string, int) ([]Project, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListMRApprovals(context.Context, string, int) ([]MRApproval, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListMRDiscussions(context.Context, string, int, *time.Time) ([]MRDiscussion, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) CreateMRDiscussionNote(context.Context, string, int, string, string) (*MRNote, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ResolveMRDiscussion(context.Context, string, int, string) error {
	return ErrNoClient
}

func (c *NoopClient) ListPipelines(context.Context, string, string) ([]Pipeline, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) GetMRFeedback(context.Context, string, int) (*MRFeedback, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) GetMRStatus(context.Context, string, int) (*MRStatus, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListMRFiles(context.Context, string, int) ([]MRFile, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListMRCommits(context.Context, string, int) ([]MRCommitInfo, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) SubmitMRApproval(context.Context, string, int) error {
	return ErrNoClient
}

func (c *NoopClient) SubmitMRUnapproval(context.Context, string, int) error {
	return ErrNoClient
}

func (c *NoopClient) CreateMR(context.Context, string, string, string, string, string, bool) (*MR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListProjectBranches(context.Context, string) ([]RepoBranch, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) ListIssuesPaged(context.Context, string, string, int, int) (*IssueSearchPage, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) SearchMRs(context.Context, string, string) ([]*MR, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) SearchMRsPaged(context.Context, string, string, int, int) (*MRSearchPage, error) {
	return nil, ErrNoClient
}

func (c *NoopClient) GetIssueState(context.Context, string, int) (string, error) {
	return "", ErrNoClient
}
