package github

import (
	"context"
	"testing"
)

type mentionSearchClient struct {
	*MockClient
	issueQuery string
	prQuery    string
	issues     []*Issue
	prs        []*PR
}

func (c *mentionSearchClient) ListIssuesPaged(
	_ context.Context,
	_, customQuery string,
	page, perPage int,
) (*IssueSearchPage, error) {
	c.issueQuery = customQuery
	return &IssueSearchPage{Issues: c.issues, TotalCount: len(c.issues), Page: page, PerPage: perPage}, nil
}

func (c *mentionSearchClient) SearchPRsPaged(
	_ context.Context,
	_, customQuery string,
	page, perPage int,
) (*PRSearchPage, error) {
	c.prQuery = customQuery
	return &PRSearchPage{PRs: c.prs, TotalCount: len(c.prs), Page: page, PerPage: perPage}, nil
}

func TestServiceSearchMentionsUsesServerBuiltTitleOnlyQuery(t *testing.T) {
	client := &mentionSearchClient{
		MockClient: NewMockClient(),
		issues:     []*Issue{{ID: 101, NodeID: "I_kwDOA", Number: 7, Title: "Fix auth", RepoOwner: "acme", RepoName: "web"}},
		prs:        []*PR{{ID: 202, NodeID: "PR_kwDOB", Number: 8, Title: "Fix auth flow", RepoOwner: "acme", RepoName: "web"}},
	}
	service := &Service{client: client, searchCache: newTTLCache()}

	issues, err := service.SearchMentionIssuesForWorkspace(
		context.Background(),
		"workspace-1",
		`fix repo:foreign/private "auth" \\ flow`,
		3,
	)
	if err != nil {
		t.Fatalf("search issue mentions: %v", err)
	}
	prs, err := service.SearchMentionPullRequestsForWorkspace(
		context.Background(),
		"workspace-1",
		`fix repo:foreign/private "auth" \\ flow`,
		4,
	)
	if err != nil {
		t.Fatalf("search PR mentions: %v", err)
	}
	wantQuery := `in:title "fix repo:foreign/private \"auth\" \\\\ flow"`
	if client.issueQuery != wantQuery || client.prQuery != wantQuery {
		t.Fatalf("queries = issue %q PR %q, want %q", client.issueQuery, client.prQuery, wantQuery)
	}
	if len(issues) != 1 || issues[0].NodeID != "I_kwDOA" {
		t.Fatalf("issues = %#v", issues)
	}
	if len(prs) != 1 || prs[0].NodeID != "PR_kwDOB" {
		t.Fatalf("PRs = %#v", prs)
	}
}

func TestServiceSearchMentionsRequiresExplicitWorkspaceAndQuery(t *testing.T) {
	service := &Service{client: NewMockClient(), searchCache: newTTLCache()}
	if _, err := service.SearchMentionIssuesForWorkspace(context.Background(), "", "auth", 5); err == nil {
		t.Fatal("blank workspace was accepted")
	}
	if _, err := service.SearchMentionPullRequestsForWorkspace(context.Background(), "workspace-1", " ", 5); err == nil {
		t.Fatal("blank query was accepted")
	}
}
