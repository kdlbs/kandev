package azuredevops

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type mentionClient struct {
	*invalidClient
	projects        []Project
	repositories    map[string][]Repository
	workItems       map[string][]WorkItem
	pullRequests    map[string][]PullRequest
	wiqlCalls       []mentionWIQLCall
	prCalls         []PullRequestFilter
	projectCalls    int
	repositoryCalls int
}

type mentionWIQLCall struct {
	projectID string
	wiql      string
	top       int
}

func (c *mentionClient) ListProjects(context.Context) ([]Project, error) {
	c.projectCalls++
	return append([]Project(nil), c.projects...), nil
}

func (c *mentionClient) ListRepositories(_ context.Context, projectID string) ([]Repository, error) {
	c.repositoryCalls++
	return append([]Repository(nil), c.repositories[projectID]...), nil
}

func (c *mentionClient) QueryWIQL(_ context.Context, projectID, wiql string, top int) (*WorkItemSearchResult, error) {
	c.wiqlCalls = append(c.wiqlCalls, mentionWIQLCall{projectID: projectID, wiql: wiql, top: top})
	items := append([]WorkItem(nil), c.workItems[projectID]...)
	if len(items) > top {
		items = items[:top]
	}
	return &WorkItemSearchResult{Items: items, Count: len(items)}, nil
}

func (c *mentionClient) ListPullRequests(_ context.Context, filter PullRequestFilter) (*PullRequestPage, error) {
	c.prCalls = append(c.prCalls, filter)
	items := append([]PullRequest(nil), c.pullRequests[filter.ProjectID]...)
	if filter.Skip >= len(items) {
		return &PullRequestPage{Skip: filter.Skip, Top: filter.Top}, nil
	}
	items = items[filter.Skip:]
	if filter.Top > 0 && len(items) > filter.Top {
		items = items[:filter.Top]
	}
	return &PullRequestPage{Items: items, Count: len(items), Skip: filter.Skip, Top: filter.Top}, nil
}

func TestServiceMentionSearchBuildsSafeWIQLAndProjectLevelPRQuery(t *testing.T) {
	client := &mentionClient{
		invalidClient: &invalidClient{},
		workItems: map[string][]WorkItem{
			"project-1": {{ID: 101, Title: "Fix auth", Project: "Platform"}},
		},
		pullRequests: map[string][]PullRequest{
			"project-1": {
				{ID: 42, Title: "Fix auth flow", ProjectID: "project-1", ProjectName: "Platform", RepositoryID: "repo-1", RepositoryName: "widgets"},
				{ID: 43, Title: "Unrelated", ProjectID: "project-1", ProjectName: "Platform", RepositoryID: "repo-1", RepositoryName: "widgets"},
			},
		},
	}
	service, _, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "workspace-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", DefaultProjectID: "project-1",
		DefaultProjectName: "Platform", PAT: "secret",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	query := `auth' OR [System.State] <> ''`
	workItems, err := service.SearchMentionWorkItemsForWorkspace(t.Context(), "workspace-1", query, 5)
	if err != nil {
		t.Fatalf("search work items: %v", err)
	}
	prs, err := service.SearchMentionPullRequestsForWorkspace(t.Context(), "workspace-1", "AUTH", 5)
	if err != nil {
		t.Fatalf("search pull requests: %v", err)
	}
	if len(client.wiqlCalls) != 1 || client.wiqlCalls[0].projectID != "project-1" {
		t.Fatalf("WIQL calls = %#v", client.wiqlCalls)
	}
	wiql := client.wiqlCalls[0].wiql
	if !strings.Contains(wiql, `CONTAINS 'auth'' OR [System.State] <> '''`) ||
		strings.Contains(wiql, `CONTAINS 'auth' OR`) {
		t.Fatalf("unsafe WIQL = %q", wiql)
	}
	if len(client.prCalls) != 1 || client.prCalls[0].RepositoryID != "" ||
		client.prCalls[0].ProjectID != "project-1" || client.prCalls[0].Status != "active" {
		t.Fatalf("PR calls = %#v", client.prCalls)
	}
	if len(workItems) != 1 || workItems[0].ID != 101 || workItems[0].OrganizationURL != "https://dev.azure.com/acme" {
		t.Fatalf("work items = %#v", workItems)
	}
	if len(prs) != 1 || prs[0].ID != 42 || prs[0].RepositoryID != "repo-1" {
		t.Fatalf("PRs = %#v", prs)
	}
}

func TestServiceMentionSearchBoundsProjectDiscovery(t *testing.T) {
	client := &mentionClient{
		invalidClient: &invalidClient{},
		workItems:     make(map[string][]WorkItem),
	}
	for index := 7; index >= 1; index-- {
		id := fmt.Sprintf("project-%d", index)
		client.projects = append(client.projects, Project{ID: id, Name: fmt.Sprintf("Project %d", index)})
		client.workItems[id] = []WorkItem{{ID: index, Title: "Match", Project: id}}
	}
	service, _, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "workspace-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "secret",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	items, err := service.SearchMentionWorkItemsForWorkspace(t.Context(), "workspace-1", "match", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if client.projectCalls != 1 || len(client.wiqlCalls) != maxMentionProjects {
		t.Fatalf("project calls = %d, WIQL calls = %d", client.projectCalls, len(client.wiqlCalls))
	}
	if len(items) != maxMentionProjects || items[0].ProjectID != "project-1" {
		t.Fatalf("items = %#v", items)
	}
}

func TestServiceMentionPullRequestSearchPaginatesUntilMatch(t *testing.T) {
	pullRequests := make([]PullRequest, mentionPRFetchLimit+1)
	for index := range pullRequests {
		pullRequests[index] = PullRequest{
			ID: index + 1, Title: "Unrelated", Status: activePullRequestState,
			ProjectID: "project-1", ProjectName: "Platform",
			RepositoryID: "repo-1", RepositoryName: "widgets",
		}
	}
	pullRequests[mentionPRFetchLimit].Title = "Needle after first page"
	client := &mentionClient{
		invalidClient: &invalidClient{},
		pullRequests:  map[string][]PullRequest{"project-1": pullRequests},
	}
	service, _, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "workspace-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", DefaultProjectID: "project-1",
		DefaultProjectName: "Platform", PAT: "secret",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	items, err := service.SearchMentionPullRequestsForWorkspace(t.Context(), "workspace-1", "needle", 1)
	if err != nil {
		t.Fatalf("search pull requests: %v", err)
	}
	if len(items) != 1 || items[0].ID != mentionPRFetchLimit+1 {
		t.Fatalf("items = %#v, want matching pull request after first page", items)
	}
	if len(client.prCalls) != 2 || client.prCalls[0].Skip != 0 || client.prCalls[0].Top != mentionPRFetchLimit ||
		client.prCalls[1].Skip != mentionPRFetchLimit || client.prCalls[1].Top != mentionPRFetchLimit {
		t.Fatalf("PR calls = %#v", client.prCalls)
	}
}

func TestServiceResolveMentionScopeRejectsForeignWorkspaceProject(t *testing.T) {
	client := &mentionClient{
		invalidClient: &invalidClient{},
		projects:      []Project{{ID: "project-1", Name: "Platform"}},
		repositories: map[string][]Repository{
			"project-1": {{ID: "repo-1", Name: "widgets", ProjectID: "project-1", ProjectName: "Platform"}},
		},
	}
	service, _, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "workspace-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "secret",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	project, err := service.ResolveMentionProjectForWorkspace(t.Context(), "workspace-1", "project-1")
	if err != nil || project.ProjectName != "Platform" {
		t.Fatalf("resolve project = %#v, %v", project, err)
	}
	repository, err := service.ResolveMentionRepositoryForWorkspace(t.Context(), "workspace-1", "project-1", "repo-1")
	if err != nil || repository.RepositoryName != "widgets" {
		t.Fatalf("resolve repository = %#v, %v", repository, err)
	}
	if _, err := service.ResolveMentionProjectForWorkspace(t.Context(), "workspace-2", "project-1"); err == nil {
		t.Fatal("foreign workspace project resolved")
	}
}
