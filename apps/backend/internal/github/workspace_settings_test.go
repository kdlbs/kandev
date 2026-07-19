package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	assertSearchCalls(t, client.calls, "is:open", []string{"repo:kdlbs/kandev", "repo:kdlbs/docs"})
	client.calls = nil

	if _, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "review-requested:@me", "", 1, 25); err != nil {
		t.Fatalf("search scoped prs with preset filter: %v", err)
	}
	assertSearchCalls(t, client.calls, "review-requested:@me", []string{"repo:kdlbs/kandev", "repo:kdlbs/docs"})
}

func assertSearchCalls(t *testing.T, calls []searchCall, baseQuery string, qualifiers []string) {
	t.Helper()
	if len(calls) != len(qualifiers) {
		t.Fatalf("search calls = %+v, want %d calls", calls, len(qualifiers))
	}
	for i, qualifier := range qualifiers {
		query := strings.TrimSpace(calls[i].filter + " " + calls[i].customQuery)
		if !strings.Contains(query, baseQuery) || !strings.Contains(query, qualifier) {
			t.Fatalf("search call %d query = %q, want %q and %q", i, query, baseQuery, qualifier)
		}
		if strings.Contains(query, " OR ") || strings.ContainsAny(query, "()") {
			t.Fatalf("search call %d should not OR qualifiers: %q", i, query)
		}
	}
}

func assertHasSearchCall(t *testing.T, calls []searchCall, qualifier string, page, perPage int) {
	t.Helper()
	for _, call := range calls {
		query := strings.TrimSpace(call.filter + " " + call.customQuery)
		if strings.Contains(query, qualifier) && call.page == page && call.perPage == perPage {
			return
		}
	}
	t.Fatalf("search calls = %+v, want %s page=%d perPage=%d", calls, qualifier, page, perPage)
}

func TestService_SearchUserPRsPagedForWorkspace_OrgScopeIncludesPersonalOwner(t *testing.T) {
	client := &capturingSearchClient{MockClient: NewMockClient()}
	client.AddPR(&PR{RepoOwner: "octo", RepoName: "personal", Number: 1, Title: "in scope"})
	store := newTestStore(t)
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	assertSearchCalls(t, client.calls, "is:open", []string{"org:octo", "user:octo"})
}

func TestService_SearchUserPRsPagedForWorkspace_FanoutFetchesDeeperProviderPages(t *testing.T) {
	client := &capturingSearchClient{MockClient: NewMockClient()}
	for i := 1; i <= 150; i++ {
		client.AddPR(&PR{RepoOwner: "kdlbs", RepoName: "kandev", Number: i, Title: fmt.Sprintf("kandev-%d", i)})
	}
	for i := 1; i <= 10; i++ {
		client.AddPR(&PR{RepoOwner: "example", RepoName: "api", Number: i, Title: fmt.Sprintf("api-%d", i)})
	}
	store := newTestStore(t)
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{
			{Owner: "kdlbs", Name: "kandev"},
			{Owner: "example", Name: "api"},
		},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	page, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "", "is:open", 5, 25)
	if err != nil {
		t.Fatalf("search scoped prs: %v", err)
	}
	if page.TotalCount != 160 {
		t.Fatalf("total count = %d, want 160", page.TotalCount)
	}
	if len(page.PRs) != 25 {
		t.Fatalf("page PR count = %d, want 25", len(page.PRs))
	}
	if page.PRs[0].RepoOwner != "kdlbs" || page.PRs[0].RepoName != "kandev" || page.PRs[0].Number != 101 {
		t.Fatalf("first PR on page = %+v, want kdlbs/kandev#101", page.PRs[0])
	}
	if page.PRs[24].RepoOwner != "kdlbs" || page.PRs[24].RepoName != "kandev" || page.PRs[24].Number != 125 {
		t.Fatalf("last PR on page = %+v, want kdlbs/kandev#125", page.PRs[24])
	}
	assertHasSearchCall(t, client.calls, "repo:kdlbs/kandev", 2, 100)
}

func TestService_SearchUserPRsPagedForWorkspace_OrgScopeDedupesFanoutTotal(t *testing.T) {
	client := &capturingSearchClient{MockClient: NewMockClient()}
	for i := 1; i <= 120; i++ {
		client.AddPR(&PR{RepoOwner: "octo", RepoName: "repo", Number: i, Title: fmt.Sprintf("octo-%d", i)})
	}
	store := newTestStore(t)
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeOrgs,
		RepoScopeOrgs: []string{"octo"},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	page, err := svc.SearchUserPRsPagedForWorkspace(ctx, "ws-1", "", "is:open", 1, 25)
	if err != nil {
		t.Fatalf("search scoped prs: %v", err)
	}
	if page.TotalCount != 120 {
		t.Fatalf("total count = %d, want 120", page.TotalCount)
	}
	if len(page.PRs) != 25 {
		t.Fatalf("page PR count = %d, want 25", len(page.PRs))
	}
	assertSearchCalls(t, client.calls, "is:open", []string{"org:octo", "user:octo"})
}

func TestService_SearchUserPRsPagedForWorkspace_EmptyRepoScopeFailsClosed(t *testing.T) {
	client := NewMockClient()
	client.AddPR(&PR{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "hidden"})
	store := newTestStore(t)
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, NewMockClient(), store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, NewMockClient(), store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, NewMockClient(), store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")

	page, err := svc.SearchUserIssuesPagedForWorkspace(context.Background(), "ws-1", "", "is:open", 1, 25)
	if err != nil {
		t.Fatalf("search scoped issues: %v", err)
	}
	if page.TotalCount != 2 || len(page.Issues) != 2 {
		t.Fatalf("all scope should preserve results, got total=%d issues=%+v", page.TotalCount, page.Issues)
	}
}

func TestService_SearchUserIssuesPagedForWorkspace_FansOutSelectedRepos(t *testing.T) {
	client := &rejectingORIssueSearchClient{
		MockClient: NewMockClient(),
		issuesByRepo: map[string][]*Issue{
			"kdlbs/kandev": {
				{RepoOwner: "kdlbs", RepoName: "kandev", Number: 1, Title: "one"},
			},
			"example/api": {
				{RepoOwner: "example", RepoName: "api", Number: 2, Title: "two"},
			},
		},
	}
	store := newTestStore(t)
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
	ctx := context.Background()

	if err := svc.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:   "ws-1",
		RepoScopeMode: RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{
			{Owner: "kdlbs", Name: "kandev"},
			{Owner: "example", Name: "api"},
		},
	}); err != nil {
		t.Fatalf("save workspace settings: %v", err)
	}

	page, err := svc.SearchUserIssuesPagedForWorkspace(ctx, "ws-1", "", "is:open", 1, 25)
	if err != nil {
		t.Fatalf("search scoped issues: %v", err)
	}
	if page.TotalCount != 2 || len(page.Issues) != 2 {
		t.Fatalf("expected both selected repo issues, got total=%d issues=%+v", page.TotalCount, page.Issues)
	}
	if len(client.calls) != 2 {
		t.Fatalf("expected one provider search per selected repo, got calls=%+v", client.calls)
	}
	for _, call := range client.calls {
		query := strings.TrimSpace(call.filter + " " + call.customQuery)
		if strings.Contains(query, " OR ") || strings.ContainsAny(query, "()") {
			t.Fatalf("provider query should not OR repo qualifiers: %q", query)
		}
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
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	svc := newWorkspaceAuthenticatedTestService(t, client, store, "ws-1")
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
	calls       []searchCall
}

type searchCall struct {
	filter      string
	customQuery string
	page        int
	perPage     int
}

type rejectingORIssueSearchClient struct {
	*MockClient
	issuesByRepo map[string][]*Issue
	calls        []searchCall
}

func (c *capturingSearchClient) SearchPRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*PRSearchPage, error) {
	c.filter = filter
	c.customQuery = customQuery
	c.calls = append(c.calls, searchCall{filter: filter, customQuery: customQuery, page: page, perPage: perPage})
	result, err := c.MockClient.SearchPRsPaged(ctx, filter, customQuery, page, perPage)
	if err != nil {
		return nil, err
	}
	return filterPRSearchPageByQuery(result, strings.TrimSpace(filter+" "+customQuery), page, perPage), nil
}

func (c *issueSearchClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	return c.issues, nil
}

func (c *issueSearchClient) ListIssuesPaged(context.Context, string, string, int, int) (*IssueSearchPage, error) {
	return &IssueSearchPage{Issues: c.issues, TotalCount: len(c.issues), Page: 1, PerPage: 25}, nil
}

func (c *rejectingORIssueSearchClient) ListIssuesPaged(_ context.Context, filter, customQuery string, page, perPage int) (*IssueSearchPage, error) {
	c.calls = append(c.calls, searchCall{filter: filter, customQuery: customQuery, page: page, perPage: perPage})
	query := strings.TrimSpace(filter + " " + customQuery)
	if strings.Contains(query, " OR ") || strings.ContainsAny(query, "()") {
		return nil, fmt.Errorf("github rejected invalid qualifier query %q", query)
	}
	for repo, issues := range c.issuesByRepo {
		if strings.Contains(query, "repo:"+repo) {
			return &IssueSearchPage{Issues: paginateSearchResults(issues, page, perPage), TotalCount: len(issues), Page: page, PerPage: perPage}, nil
		}
	}
	return &IssueSearchPage{Issues: []*Issue{}, TotalCount: 0, Page: page, PerPage: perPage}, nil
}

func filterPRSearchPageByQuery(page *PRSearchPage, query string, pageNum, perPage int) *PRSearchPage {
	if page == nil {
		return nil
	}
	if !strings.Contains(query, "repo:") && !strings.Contains(query, "org:") && !strings.Contains(query, "user:") {
		return page
	}
	prs := make([]*PR, 0, len(page.PRs))
	for _, pr := range page.PRs {
		if prMatchesQueryQualifier(pr, query) {
			prs = append(prs, pr)
		}
	}
	sort.Slice(prs, func(i, j int) bool {
		left := strings.ToLower(prs[i].RepoOwner + "/" + prs[i].RepoName)
		right := strings.ToLower(prs[j].RepoOwner + "/" + prs[j].RepoName)
		if left != right {
			return left < right
		}
		return prs[i].Number < prs[j].Number
	})
	return &PRSearchPage{PRs: paginateSearchResults(prs, pageNum, perPage), TotalCount: len(prs), Page: pageNum, PerPage: perPage}
}

func prMatchesQueryQualifier(pr *PR, query string) bool {
	if pr == nil {
		return false
	}
	owner := strings.ToLower(strings.TrimSpace(pr.RepoOwner))
	name := strings.ToLower(strings.TrimSpace(pr.RepoName))
	query = strings.ToLower(query)
	return strings.Contains(query, fmt.Sprintf("repo:%s/%s", owner, name)) ||
		strings.Contains(query, "org:"+owner) ||
		strings.Contains(query, "user:"+owner)
}
