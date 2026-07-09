package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestStore_GitHubWorkspaceSettingsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	input := &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeRepos,
		RepoScopeOrgs: []string{"kdlbs"},
		RepoScopeRepos: []RepoFilter{
			{Owner: "kdlbs", Name: "kandev"},
		},
		SavedPresets:        []byte(`[{"id":"p1","kind":"pr","label":"Mine"}]`),
		DefaultQueryPresets: []byte(`{"pr":[],"issue":[]}`),
	}

	if err := store.UpsertWorkspaceSettings(ctx, input); err != nil {
		t.Fatalf("upsert workspace settings: %v", err)
	}

	got, err := store.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get workspace settings: %v", err)
	}
	if got.WorkspaceID != "ws-1" || got.RepoScopeMode != RepoScopeModeRepos {
		t.Fatalf("unexpected settings identity/scope: %+v", got)
	}
	if len(got.RepoScopeRepos) != 1 || got.RepoScopeRepos[0].Owner != "kdlbs" || got.RepoScopeRepos[0].Name != "kandev" {
		t.Fatalf("repo scope lost on round trip: %+v", got.RepoScopeRepos)
	}
	if string(got.SavedPresets) != string(input.SavedPresets) {
		t.Fatalf("saved presets = %s, want %s", got.SavedPresets, input.SavedPresets)
	}
	if string(got.DefaultQueryPresets) != string(input.DefaultQueryPresets) {
		t.Fatalf("default query presets = %s, want %s", got.DefaultQueryPresets, input.DefaultQueryPresets)
	}
}

func TestService_SearchUserPRsPagedForWorkspace_FiltersToSelectedRepos(t *testing.T) {
	client := NewMockClient()
	client.AddPR(&PR{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "in scope"})
	client.AddPR(&PR{RepoOwner: "other", RepoName: "repo", Number: 2, Title: "out of scope"})
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:    "ws-1",
		RepoScopeMode:  RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{{Owner: "kdlbs", Name: "kandev"}},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	page, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "", "repo:other/repo is:open", 1, 25)
	if err != nil {
		t.Fatalf("search scoped prs: %v", err)
	}
	if page.TotalCount != 2 || len(page.PRs) != 1 {
		t.Fatalf("expected one scoped PR, got total=%d prs=%+v", page.TotalCount, page.PRs)
	}
	if page.PRs[0].RepoOwner != "kdlbs" || page.PRs[0].RepoName != "kandev" {
		t.Fatalf("custom query escaped workspace scope: %+v", page.PRs[0])
	}
}

func TestService_SearchUserPRsPagedForWorkspace_AppendsScopeToQuery(t *testing.T) {
	client := &capturingSearchClient{MockClient: NewMockClient()}
	client.AddPR(&PR{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "in scope"})
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{
			{Owner: "kdlbs", Name: "kandev"},
			{Owner: "kdlbs", Name: "docs"},
		},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	page, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "", "is:open", 1, 25)
	if err != nil {
		t.Fatalf("search scoped prs: %v", err)
	}
	if page.TotalCount != 1 || len(page.PRs) != 1 {
		t.Fatalf("expected one scoped PR, got total=%d prs=%+v", page.TotalCount, page.PRs)
	}
	if !strings.Contains(client.customQuery, "is:open") ||
		!strings.Contains(client.customQuery, "(repo:kdlbs/kandev OR repo:kdlbs/docs)") {
		t.Fatalf("custom query was not workspace scoped: %q", client.customQuery)
	}

	if _, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "review-requested:@me", "", 1, 25); err != nil {
		t.Fatalf("search scoped prs with preset filter: %v", err)
	}
	if !strings.Contains(client.filter, "review-requested:@me") ||
		!strings.Contains(client.filter, "(repo:kdlbs/kandev OR repo:kdlbs/docs)") {
		t.Fatalf("filter query was not workspace scoped: %q", client.filter)
	}
}

func TestService_SearchUserPRsPagedForWorkspace_OrgScopeIncludesPersonalOwner(t *testing.T) {
	client := &capturingSearchClient{MockClient: NewMockClient()}
	client.AddPR(&PR{RepoOwner: "octo", RepoName: "personal", Number: 1, Title: "in scope"})
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeOrgs,
		RepoScopeOrgs: []string{"octo"},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	if _, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "", "is:open", 1, 25); err != nil {
		t.Fatalf("search scoped prs: %v", err)
	}
	if !strings.Contains(client.customQuery, "(org:octo OR user:octo)") {
		t.Fatalf("org scope did not include personal-owner qualifier: %q", client.customQuery)
	}
}

func TestService_SearchUserPRsPagedForWorkspace_EmptyRepoScopeFailsClosed(t *testing.T) {
	client := NewMockClient()
	client.AddPR(&PR{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "hidden"})
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:    "ws-1",
		RepoScopeMode:  RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	page, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "", "is:open", 1, 25)
	if err != nil {
		t.Fatalf("search scoped prs: %v", err)
	}
	if page.TotalCount != 0 || len(page.PRs) != 0 {
		t.Fatalf("empty repo scope should return no PRs, got total=%d prs=%+v", page.TotalCount, page.PRs)
	}
}

func TestService_UpdateWorkspaceSettings_PartialUpdateAndNullClear(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(NewMockClient(), AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()
	defaults := json.RawMessage(`{"pr":[{"value":"mine","label":"Mine","filter":"is:open","group":"inbox"}],"issue":[]}`)
	saved := json.RawMessage(`[{"id":"p1","kind":"pr","label":"Mine"}]`)
	mode := RepoScopeModeRepos
	repos := []RepoFilter{{Owner: "kdlbs", Name: "kandev"}}
	if _, err := svc.UpdateWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID:         "ws-1",
		RepoScopeMode:       &mode,
		RepoScopeRepos:      &repos,
		SavedPresets:        &saved,
		SavedPresetsSet:     true,
		DefaultQueryPresets: &defaults,
		DefaultQueriesSet:   true,
	}); err != nil {
		t.Fatalf("initial update: %v", err)
	}

	nextSaved := json.RawMessage(`[{"id":"p2","kind":"issue","label":"Issues"}]`)
	got, err := svc.UpdateWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID:     "ws-1",
		SavedPresets:    &nextSaved,
		SavedPresetsSet: true,
	})
	if err != nil {
		t.Fatalf("partial saved presets update: %v", err)
	}
	if string(got.SavedPresets) != string(nextSaved) {
		t.Fatalf("saved presets = %s, want %s", got.SavedPresets, nextSaved)
	}
	if string(got.DefaultQueryPresets) != string(defaults) {
		t.Fatalf("default presets should be preserved, got %s", got.DefaultQueryPresets)
	}

	got, err = svc.UpdateWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID:       "ws-1",
		DefaultQueriesSet: true,
	})
	if err != nil {
		t.Fatalf("clear default query presets: %v", err)
	}
	if got.DefaultQueryPresets != nil {
		t.Fatalf("default presets should be cleared, got %s", got.DefaultQueryPresets)
	}
	if string(got.SavedPresets) != string(nextSaved) {
		t.Fatalf("saved presets should be preserved after clear, got %s", got.SavedPresets)
	}

	got, err = svc.UpdateWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID:     "ws-1",
		SavedPresetsSet: true,
	})
	if err != nil {
		t.Fatalf("clear saved presets: %v", err)
	}
	if string(got.SavedPresets) != "[]" {
		t.Fatalf("saved presets should be cleared to empty array, got %s", got.SavedPresets)
	}
}

func TestUpdateWorkspaceSettingsRequest_UnmarshalTracksExplicitNull(t *testing.T) {
	var req UpdateWorkspaceSettingsRequest
	if err := json.Unmarshal([]byte(`{"workspace_id":"ws-1","default_query_presets":null}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if !req.DefaultQueriesSet {
		t.Fatal("expected default query presets to be marked present")
	}
	if req.DefaultQueryPresets != nil {
		t.Fatalf("expected explicit null to leave raw pointer nil, got %s", *req.DefaultQueryPresets)
	}
}

func TestUpdateWorkspaceSettingsRequest_UnmarshalTracksSavedPresetsNull(t *testing.T) {
	var req UpdateWorkspaceSettingsRequest
	if err := json.Unmarshal([]byte(`{"workspace_id":"ws-1","saved_presets":null}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if !req.SavedPresetsSet {
		t.Fatal("expected saved presets to be marked present")
	}
	if req.SavedPresets != nil {
		t.Fatalf("expected explicit null to leave raw pointer nil, got %s", *req.SavedPresets)
	}
}

func TestService_UpdateWorkspaceSettings_RejectsUnknownRepoScopeMode(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(NewMockClient(), AuthMethodPAT, nil, store, nil, testLogger(t))
	mode := "everything"

	_, err := svc.UpdateWorkspaceSettings(context.Background(), &UpdateWorkspaceSettingsRequest{
		WorkspaceID:   "ws-1",
		RepoScopeMode: &mode,
	})
	if !errors.Is(err, ErrWorkspaceSettingsValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestService_UpdateWorkspaceSettings_RejectsConflictingScopePatch(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(NewMockClient(), AuthMethodPAT, nil, store, nil, testLogger(t))
	mode := RepoScopeModeAll
	orgs := []string{"octo"}

	_, err := svc.UpdateWorkspaceSettings(context.Background(), &UpdateWorkspaceSettingsRequest{
		WorkspaceID:   "ws-1",
		RepoScopeMode: &mode,
		RepoScopeOrgs: &orgs,
	})
	if !errors.Is(err, ErrWorkspaceSettingsValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestService_SearchUserIssuesPagedForWorkspace_AllScopePreservesResults(t *testing.T) {
	client := &issueSearchClient{
		issues: []*Issue{
			{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "one"},
			{RepoOwner: "other", RepoName: "repo", Number: 2, Title: "two"},
		},
	}
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))

	page, err := svc.SearchUserIssuesPagedForWorkspace(context.Background(), "ws-1", "", "is:open", 1, 25)
	if err != nil {
		t.Fatalf("search scoped issues: %v", err)
	}
	if page.TotalCount != 2 || len(page.Issues) != 2 {
		t.Fatalf("all scope should preserve results, got total=%d issues=%+v", page.TotalCount, page.Issues)
	}
}

func TestService_CheckReviewWatch_AppliesWorkspaceRepoScope(t *testing.T) {
	client := NewMockClient()
	client.AddPR(&PR{
		RepoOwner:          "kdlbs",
		RepoName:           "kandev",
		Number:             1,
		Title:              "in scope",
		RequestedReviewers: []RequestedReviewer{{Login: "octo", Type: "user"}},
	})
	client.AddPR(&PR{
		RepoOwner:          "other",
		RepoName:           "repo",
		Number:             2,
		Title:              "out of scope",
		RequestedReviewers: []RequestedReviewer{{Login: "octo", Type: "user"}},
	})
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:    "ws-1",
		RepoScopeMode:  RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{{Owner: "kdlbs", Name: "kandev"}},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	prs, err := svc.CheckReviewWatch(ctx, &ReviewWatch{
		ID:                  "watch-1",
		WorkspaceID:         "ws-1",
		Repos:               nil,
		ReviewScope:         ReviewScopeUserAndTeams,
		Enabled:             true,
		PollIntervalSeconds: 300,
	})
	if err != nil {
		t.Fatalf("check review watch: %v", err)
	}
	if len(prs) != 1 || prs[0].RepoOwner != "kdlbs" || prs[0].RepoName != "kandev" {
		t.Fatalf("expected one in-scope PR, got %+v", prs)
	}
}

func TestService_CheckReviewWatch_EmptyWorkspaceScopeSkipsProviderFetch(t *testing.T) {
	client := &countingReviewClient{MockClient: NewMockClient()}
	client.AddPR(&PR{
		RepoOwner:          "kdlbs",
		RepoName:           "kandev",
		Number:             1,
		Title:              "hidden",
		RequestedReviewers: []RequestedReviewer{{Login: "octo", Type: "user"}},
	})
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:    "ws-1",
		RepoScopeMode:  RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	prs, err := svc.CheckReviewWatch(ctx, &ReviewWatch{
		ID:                  "watch-1",
		WorkspaceID:         "ws-1",
		Repos:               nil,
		ReviewScope:         ReviewScopeUserAndTeams,
		Enabled:             true,
		PollIntervalSeconds: 300,
	})
	if err != nil {
		t.Fatalf("check review watch: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("empty workspace scope should return no PRs, got %+v", prs)
	}
	if client.listReviewRequestedCalls != 0 {
		t.Fatalf("expected no broad provider fetch, got %d calls", client.listReviewRequestedCalls)
	}
}

func TestService_CheckIssueWatch_AppliesWorkspaceRepoScope(t *testing.T) {
	client := &issueSearchClient{
		issues: []*Issue{
			{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "in scope"},
			{RepoOwner: "other", RepoName: "repo", Number: 2, Title: "out of scope"},
		},
	}
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeOrgs,
		RepoScopeOrgs: []string{"kdlbs"},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	issues, err := svc.CheckIssueWatch(ctx, &IssueWatch{
		ID:                  "watch-1",
		WorkspaceID:         "ws-1",
		Repos:               nil,
		Enabled:             true,
		PollIntervalSeconds: 300,
	})
	if err != nil {
		t.Fatalf("check issue watch: %v", err)
	}
	if len(issues) != 1 || issues[0].RepoOwner != "kdlbs" || issues[0].RepoName != "kandev" {
		t.Fatalf("expected one in-scope issue, got %+v", issues)
	}
}

func TestService_CheckIssueWatch_EmptyWorkspaceScopeSkipsProviderFetch(t *testing.T) {
	client := &countingWorkspaceIssueClient{MockClient: NewMockClient()}
	store := newTestStore(t)
	svc := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeOrgs,
		RepoScopeOrgs: []string{},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	issues, err := svc.CheckIssueWatch(ctx, &IssueWatch{
		ID:                  "watch-1",
		WorkspaceID:         "ws-1",
		Repos:               nil,
		Enabled:             true,
		PollIntervalSeconds: 300,
	})
	if err != nil {
		t.Fatalf("check issue watch: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("empty workspace scope should return no issues, got %+v", issues)
	}
	if client.listIssuesCalls != 0 {
		t.Fatalf("expected no broad provider fetch, got %d calls", client.listIssuesCalls)
	}
}

type issueSearchClient struct {
	*MockClient
	issues []*Issue
}

type countingReviewClient struct {
	*MockClient
	listReviewRequestedCalls int
}

func (c *countingReviewClient) ListReviewRequestedPRs(ctx context.Context, reviewScope, owner, repo string) ([]*PR, error) {
	c.listReviewRequestedCalls++
	return c.MockClient.ListReviewRequestedPRs(ctx, reviewScope, owner, repo)
}

type countingWorkspaceIssueClient struct {
	*MockClient
	listIssuesCalls int
}

func (c *countingWorkspaceIssueClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	c.listIssuesCalls++
	return []*Issue{{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "hidden"}}, nil
}

type capturingSearchClient struct {
	*MockClient
	filter      string
	customQuery string
}

func (c *capturingSearchClient) SearchPRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*PRSearchPage, error) {
	c.filter = filter
	c.customQuery = customQuery
	return c.MockClient.SearchPRsPaged(ctx, filter, customQuery, page, perPage)
}

func (c *issueSearchClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	return c.issues, nil
}

func (c *issueSearchClient) ListIssuesPaged(context.Context, string, string, int, int) (*IssueSearchPage, error) {
	return &IssueSearchPage{Issues: c.issues, TotalCount: len(c.issues), Page: 1, PerPage: 25}, nil
}
