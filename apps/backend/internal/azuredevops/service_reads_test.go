package azuredevops

import (
	"context"
	"errors"
	"testing"
)

// readsClient is a fully programmable Client used by the workspace read
// surface tests: every method can be pointed at a canned result or a failure
// so each error hop can be asserted independently.
type readsClient struct {
	invalidClient
	auth            *TestConnectionResult
	authErr         error
	repositories    []Repository
	repositoriesErr error
	branches        []Branch
	branchesErr     error
	lastBranchArgs  [2]string
	workItem        *WorkItem
	workItemErr     error
	search          *WorkItemSearchResult
	searchErr       error
	lastSearch      [3]any
	page            *PullRequestPage
	pageErr         error
	lastFilter      PullRequestFilter
	pr              *PullRequest
	prErr           error
	reviewers       []Reviewer
	reviewersErr    error
	threads         []Thread
	threadsErr      error
	refs            []WorkItemRef
	refsErr         error
	policies        []PolicyEvaluation
	policiesErr     error
}

func (c *readsClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	return c.auth, c.authErr
}

func (c *readsClient) ListRepositories(_ context.Context, projectID string) ([]Repository, error) {
	if c.repositoriesErr != nil {
		return nil, c.repositoriesErr
	}
	result := make([]Repository, 0, len(c.repositories))
	for _, repository := range c.repositories {
		repository.ProjectID = projectID
		result = append(result, repository)
	}
	return result, nil
}

func (c *readsClient) ListBranches(_ context.Context, projectID, repositoryID string) ([]Branch, error) {
	c.lastBranchArgs = [2]string{projectID, repositoryID}
	return c.branches, c.branchesErr
}

func (c *readsClient) GetWorkItem(context.Context, string, int) (*WorkItem, error) {
	return c.workItem, c.workItemErr
}

func (c *readsClient) QueryWIQL(_ context.Context, projectID, wiql string, top int) (*WorkItemSearchResult, error) {
	c.lastSearch = [3]any{projectID, wiql, top}
	return c.search, c.searchErr
}

func (c *readsClient) ListPullRequests(_ context.Context, filter PullRequestFilter) (*PullRequestPage, error) {
	c.lastFilter = filter
	return c.page, c.pageErr
}

func (c *readsClient) GetPullRequest(context.Context, string, string, int) (*PullRequest, error) {
	return c.pr, c.prErr
}

func (c *readsClient) ListReviewers(context.Context, string, string, int) ([]Reviewer, error) {
	return c.reviewers, c.reviewersErr
}

func (c *readsClient) ListThreads(context.Context, string, string, int) ([]Thread, error) {
	return c.threads, c.threadsErr
}

func (c *readsClient) ListLinkedWorkItems(context.Context, string, string, int) ([]WorkItemRef, error) {
	return c.refs, c.refsErr
}

func (c *readsClient) ListPolicyEvaluations(context.Context, string, int) ([]PolicyEvaluation, error) {
	return c.policies, c.policiesErr
}

func newReadsService(t *testing.T, client Client) *Service {
	t.Helper()
	service, _, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	return service
}

func TestListRepositoriesForWorkspaceScopesToProject(t *testing.T) {
	client := &readsClient{repositories: []Repository{{
		ID: "repo-1", Name: "widgets", ProjectName: "Platform",
		DefaultBranch: "main", WebURL: "https://dev.azure.com/acme/Platform/_git/widgets",
	}}}
	service := newReadsService(t, client)

	repositories, err := service.ListRepositoriesForWorkspace(t.Context(), "ws-1", "project-1")
	if err != nil {
		t.Fatalf("ListRepositoriesForWorkspace: %v", err)
	}
	want := Repository{
		ID: "repo-1", Name: "widgets", ProjectID: "project-1", ProjectName: "Platform",
		DefaultBranch: "main", WebURL: "https://dev.azure.com/acme/Platform/_git/widgets",
	}
	if len(repositories) != 1 || repositories[0] != want {
		t.Fatalf("repositories = %+v, want [%+v]", repositories, want)
	}
}

func TestListRepositoriesForWorkspaceRejectsBlankProjectAndPropagatesFailures(t *testing.T) {
	upstream := errors.New("repositories unavailable")
	client := &readsClient{repositoriesErr: upstream}
	service := newReadsService(t, client)

	if _, err := service.ListRepositoriesForWorkspace(t.Context(), "ws-1", "   "); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("blank project error = %v, want ErrInvalidConfig", err)
	}
	if _, err := service.ListRepositoriesForWorkspace(t.Context(), "ws-2", "project-1"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured workspace error = %v, want ErrNotConfigured", err)
	}
	if _, err := service.ListRepositoriesForWorkspace(t.Context(), "ws-1", "project-1"); !errors.Is(err, upstream) {
		t.Fatalf("upstream error = %v, want %v", err, upstream)
	}
}

func TestListBranchesForWorkspaceValidatesInputAndForwardsIdentifiers(t *testing.T) {
	client := &readsClient{branches: []Branch{{Name: "main"}, {Name: "feature/azure"}}}
	service := newReadsService(t, client)

	for _, tt := range []struct{ name, project, repository string }{
		{"blank project", "  ", "repo-1"},
		{"blank repository", "project-1", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.ListBranchesForWorkspace(
				t.Context(), "ws-1", "", tt.project, tt.repository,
			); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}

	branches, err := service.ListBranchesForWorkspace(t.Context(), "ws-1", "acme", "project-1", "repo-1")
	if err != nil {
		t.Fatalf("ListBranchesForWorkspace: %v", err)
	}
	if len(branches) != 2 || branches[0].Name != "main" || branches[1].Name != "feature/azure" {
		t.Fatalf("branches = %+v", branches)
	}
	if client.lastBranchArgs != [2]string{"project-1", "repo-1"} {
		t.Fatalf("client received %v, want project-1/repo-1", client.lastBranchArgs)
	}
}

func TestListBranchesForWorkspaceRejectsMalformedOrganizationAndMissingCapability(t *testing.T) {
	service := newReadsService(t, &readsClient{})
	if _, err := service.ListBranchesForWorkspace(
		t.Context(), "ws-1", "not a valid org name", "project-1", "repo-1",
	); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("malformed organization error = %v, want ErrInvalidConfig", err)
	}

	limited := newReadsService(t, &invalidClient{})
	_, err := limited.ListBranchesForWorkspace(t.Context(), "ws-1", "", "project-1", "repo-1")
	if err == nil || err.Error() != "azure devops: branch listing is unavailable" {
		t.Fatalf("error = %v, want branch listing unavailable", err)
	}
}

func TestSearchAndGetWorkItemForWorkspaceValidateArguments(t *testing.T) {
	client := &readsClient{
		search:   &WorkItemSearchResult{Items: []WorkItem{{ID: 101, Title: "Fix build"}}, Count: 1},
		workItem: &WorkItem{ID: 101, Revision: 4, Title: "Fix build", State: "Active"},
	}
	service := newReadsService(t, client)

	if _, err := service.SearchWorkItemsForWorkspace(t.Context(), "ws-1", "", "SELECT", 10); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("blank project search error = %v, want ErrInvalidConfig", err)
	}
	if _, err := service.SearchWorkItemsForWorkspace(t.Context(), "ws-1", "project-1", "  ", 10); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("blank wiql search error = %v, want ErrInvalidConfig", err)
	}
	result, err := service.SearchWorkItemsForWorkspace(t.Context(), "ws-1", "project-1", "SELECT [System.Id]", 25)
	if err != nil || result.Count != 1 || result.Items[0].ID != 101 {
		t.Fatalf("search result = %+v, %v", result, err)
	}
	if client.lastSearch != [3]any{"project-1", "SELECT [System.Id]", 25} {
		t.Fatalf("client received %v", client.lastSearch)
	}

	for _, tt := range []struct {
		name    string
		project string
		id      int
	}{
		{"blank project", "", 101},
		{"non-positive id", "project-1", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.GetWorkItemForWorkspace(t.Context(), "ws-1", tt.project, tt.id); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
	item, err := service.GetWorkItemForWorkspace(t.Context(), "ws-1", "project-1", 101)
	if err != nil || item.ID != 101 || item.Revision != 4 || item.State != "Active" {
		t.Fatalf("GetWorkItemForWorkspace = %+v, %v", item, err)
	}
	if _, err := service.GetWorkItemForWorkspace(t.Context(), "ws-2", "project-1", 101); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured workspace error = %v, want ErrNotConfigured", err)
	}
}

func TestListPullRequestsForWorkspaceRequiresProjectAndRepository(t *testing.T) {
	service := newReadsService(t, &readsClient{page: &PullRequestPage{}})
	for _, tt := range []struct{ name, project, repository string }{
		{"blank project", " ", "repo-1"},
		{"blank repository", "project-1", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.ListPullRequestsForWorkspace(t.Context(), "ws-1", PullRequestFilter{
				ProjectID: tt.project, RepositoryID: tt.repository,
			})
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestListPullRequestsForWorkspaceResolvesCurrentUserFilters(t *testing.T) {
	client := &readsClient{
		auth: &TestConnectionResult{OK: true, ID: "identity-1", DisplayName: "Ada"},
		page: &PullRequestPage{Items: []PullRequest{{ID: 42}}, Count: 1, Top: 50},
	}
	service := newReadsService(t, client)

	page, err := service.ListPullRequestsForWorkspace(t.Context(), "ws-1", PullRequestFilter{
		ProjectID: "project-1", RepositoryID: "repo-1", CreatorID: "@ME", ReviewerID: "@me",
		Status: "active", Skip: 10, Top: 20,
	})
	if err != nil {
		t.Fatalf("ListPullRequestsForWorkspace: %v", err)
	}
	if page.Count != 1 || len(page.Items) != 1 || page.Items[0].ID != 42 {
		t.Fatalf("page = %+v", page)
	}
	want := PullRequestFilter{
		ProjectID: "project-1", RepositoryID: "repo-1", Status: "active",
		CreatorID: "identity-1", ReviewerID: "identity-1", Skip: 10, Top: 20,
	}
	if client.lastFilter != want {
		t.Fatalf("filter sent upstream = %+v, want %+v", client.lastFilter, want)
	}
}

func TestListPullRequestsForWorkspaceKeepsExplicitFiltersUnresolved(t *testing.T) {
	client := &readsClient{
		auth: &TestConnectionResult{OK: true, ID: "identity-1"},
		page: &PullRequestPage{},
	}
	service := newReadsService(t, client)

	if _, err := service.ListPullRequestsForWorkspace(t.Context(), "ws-1", PullRequestFilter{
		ProjectID: "project-1", RepositoryID: "repo-1", CreatorID: "someone-else",
	}); err != nil {
		t.Fatalf("ListPullRequestsForWorkspace: %v", err)
	}
	if client.lastFilter.CreatorID != "someone-else" || client.lastFilter.ReviewerID != "" {
		t.Fatalf("filter = %+v, want the explicit creator preserved", client.lastFilter)
	}
}

func TestListPullRequestsForWorkspaceFailsWhenIdentityIsUnavailable(t *testing.T) {
	probeErr := errors.New("identity probe failed")
	tests := []struct {
		name    string
		client  *readsClient
		wantErr error
	}{
		{"probe error", &readsClient{authErr: probeErr}, probeErr},
		{"nil identity", &readsClient{}, ErrInvalidConfig},
		{"unauthenticated identity", &readsClient{auth: &TestConnectionResult{OK: false, ID: "x"}}, ErrInvalidConfig},
		{"identity without id", &readsClient{auth: &TestConnectionResult{OK: true, ID: "  "}}, ErrInvalidConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newReadsService(t, tt.client)
			_, err := service.ListPullRequestsForWorkspace(t.Context(), "ws-1", PullRequestFilter{
				ProjectID: "project-1", RepositoryID: "repo-1", ReviewerID: "@me",
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.client.lastFilter.ProjectID != "" {
				t.Fatalf("pull requests were listed despite an unusable identity: %+v", tt.client.lastFilter)
			}
		})
	}
}

func TestGetPullRequestForWorkspaceReturnsUpstreamRecord(t *testing.T) {
	client := &readsClient{pr: &PullRequest{
		ID: 42, Title: "Ship it", Status: "active", SourceBranch: "feature", TargetBranch: "main",
		ProjectID: "project-1", RepositoryID: "repo-1",
	}}
	service := newReadsService(t, client)

	pr, err := service.GetPullRequestForWorkspace(t.Context(), "ws-1", "project-1", "repo-1", 42)
	if err != nil || pr.ID != 42 || pr.Title != "Ship it" || pr.SourceBranch != "feature" {
		t.Fatalf("GetPullRequestForWorkspace = %+v, %v", pr, err)
	}
	if _, err := service.GetPullRequestForWorkspace(t.Context(), "ws-2", "project-1", "repo-1", 42); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured workspace error = %v, want ErrNotConfigured", err)
	}
}

// TestGetPullRequestFeedbackPropagatesEveryUpstreamFailure walks each of the
// five upstream calls the aggregation makes and proves a failure at any one of
// them aborts the whole read rather than returning partial feedback.
func TestGetPullRequestFeedbackPropagatesEveryUpstreamFailure(t *testing.T) {
	upstream := errors.New("upstream unavailable")
	base := func() *readsClient {
		return &readsClient{pr: &PullRequest{ID: 42}}
	}
	tests := []struct {
		name string
		fail func(*readsClient)
	}{
		{"pull request", func(c *readsClient) { c.prErr = upstream }},
		{"reviewers", func(c *readsClient) { c.reviewersErr = upstream }},
		{"threads", func(c *readsClient) { c.threadsErr = upstream }},
		{"linked work items", func(c *readsClient) { c.refsErr = upstream }},
		{"policies", func(c *readsClient) { c.policiesErr = upstream }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := base()
			tt.fail(client)
			service := newReadsService(t, client)
			feedback, err := service.GetPullRequestFeedbackForWorkspace(t.Context(), "ws-1", "project-1", "repo-1", 42)
			if !errors.Is(err, upstream) || feedback != nil {
				t.Fatalf("feedback = %+v, err = %v; want the upstream failure", feedback, err)
			}
		})
	}
}

func TestGetPullRequestFeedbackNormalizesEmptyCollections(t *testing.T) {
	client := &readsClient{pr: &PullRequest{ID: 42, Title: "Ship it"}}
	service := newReadsService(t, client)

	feedback, err := service.GetPullRequestFeedbackForWorkspace(t.Context(), "ws-1", "project-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("GetPullRequestFeedbackForWorkspace: %v", err)
	}
	if feedback.PullRequest.ID != 42 || feedback.ReviewState != "" || feedback.PolicyState != "" {
		t.Fatalf("feedback summary = %+v", feedback)
	}
	if feedback.Reviewers == nil || feedback.Threads == nil ||
		feedback.LinkedWorkItems == nil || feedback.Policies == nil {
		t.Fatalf("nil collections leak into the response: %+v", feedback)
	}
	if len(feedback.Reviewers)+len(feedback.Threads)+len(feedback.LinkedWorkItems)+len(feedback.Policies) != 0 {
		t.Fatalf("unexpected feedback content: %+v", feedback)
	}
}
