package github

import (
	"context"
	"testing"
)

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.5
// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.6
func TestWorkspaceScopeUnionSearch(t *testing.T) {
	client := &capturingSearchClient{MockClient: NewMockClient()}
	client.AddPR(&PR{RepoOwner: "acme", RepoName: "widget", Number: 1})
	client.AddPR(&PR{RepoOwner: "acme", RepoName: "docs", Number: 2})
	client.AddPR(&PR{RepoOwner: "outside", RepoName: "tool", Number: 3})
	client.AddPR(&PR{RepoOwner: "outside", RepoName: "other", Number: 4})
	store := newTestStore(t)
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-union-search")
	ctx := context.Background()
	settings := &WorkspaceSettings{
		WorkspaceID:   "ws-union-search",
		RepoScopeMode: RepoScopeModeSelected,
		RepoScopeOrgs: []string{"acme"},
		RepoScopeRepos: []RepoFilter{
			{Owner: "outside", Name: "tool"},
			{Owner: "acme", Name: "widget"},
		},
	}
	if err := svc.UpsertWorkspaceSettings(ctx, settings); err != nil {
		t.Fatalf("save scope: %v", err)
	}
	page, err := svc.SearchUserPRsPagedForWorkspace(ctx, settings.WorkspaceID, "", "is:open", 1, 25)
	if err != nil {
		t.Fatalf("search union scope: %v", err)
	}
	if page.TotalCount != 3 || len(page.PRs) != 3 {
		t.Fatalf("union search total=%d PRs=%+v, want each allowed result once", page.TotalCount, page.PRs)
	}
	assertSearchCalls(t, client.calls, "is:open", []string{"org:acme", "user:acme", "repo:outside/tool"})
	for _, pr := range page.PRs {
		if pr.RepoOwner == "outside" && pr.RepoName == "other" {
			t.Fatal("search returned a repository outside the workspace union")
		}
	}

	settings.RepoScopeRepos = []RepoFilter{{Owner: "outside", Name: "second"}}
	if err := svc.UpsertWorkspaceSettings(ctx, settings); err != nil {
		t.Fatalf("change exact repository scope: %v", err)
	}
	if _, err := svc.SearchUserPRsPagedForWorkspace(ctx, settings.WorkspaceID, "", "is:open", 1, 25); err != nil {
		t.Fatalf("search after exact repository change: %v", err)
	}
	if len(client.calls) != 6 {
		t.Fatalf("provider calls after exact repository change = %d, want fresh three-call fanout", len(client.calls))
	}

	settings.RepoScopeOrgs = []string{"other-owner"}
	if err := svc.UpsertWorkspaceSettings(ctx, settings); err != nil {
		t.Fatalf("change owner scope: %v", err)
	}
	if _, err := svc.SearchUserPRsPagedForWorkspace(ctx, settings.WorkspaceID, "", "is:open", 1, 25); err != nil {
		t.Fatalf("search after owner scope change: %v", err)
	}
	if len(client.calls) != 9 {
		t.Fatalf("provider calls after owner scope change = %d, want fresh three-call fanout", len(client.calls))
	}
}

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.7
// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.11
func TestWorkspaceScopeUnionWatch(t *testing.T) {
	settings := &WorkspaceSettings{
		RepoScopeMode:  RepoScopeModeSelected,
		RepoScopeOrgs:  []string{"acme"},
		RepoScopeRepos: []RepoFilter{{Owner: "outside", Name: "tool"}},
	}
	filters := workspaceScopeRepoFilters(settings)
	want := []RepoFilter{{Owner: "acme"}, {Owner: "outside", Name: "tool"}}
	if len(filters) != len(want) || filters[0] != want[0] || filters[1] != want[1] {
		t.Fatalf("watch filters = %+v, want workspace union %+v", filters, want)
	}
	candidates := []*PR{
		{RepoOwner: "acme", RepoName: "manual", Number: 1},
		{RepoOwner: "outside", RepoName: "tool", Number: 2},
		{RepoOwner: "outside", RepoName: "other", Number: 3},
	}
	allowed := filterPRsByWorkspaceScope(candidates, settings)
	if len(allowed) != 2 {
		t.Fatalf("watch union results = %+v, want owner and exact repository only", allowed)
	}
	intersected := filterPRsByWorkspaceScope([]*PR{
		{RepoOwner: "outside", RepoName: "other", Number: 4},
		{RepoOwner: "acme", RepoName: "explicit", Number: 5},
	}, settings)
	if len(intersected) != 1 || intersected[0].RepoName != "explicit" {
		t.Fatalf("explicit watch filter escaped workspace union: %+v", intersected)
	}
	empty := &WorkspaceSettings{RepoScopeMode: RepoScopeModeSelected}
	if !workspaceSettingsHasEmptyScope(empty) || len(workspaceScopeRepoFilters(empty)) != 0 {
		t.Fatalf("empty selected scope became unrestricted: filters=%+v", workspaceScopeRepoFilters(empty))
	}
}
