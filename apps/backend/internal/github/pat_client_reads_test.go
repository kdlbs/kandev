package github

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPATClient_SetsAuthAndVersionHeadersOnGET(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{"/user": `{"login":"octocat"}`})

	if _, err := c.GetAuthenticatedUser(context.Background()); err != nil {
		t.Fatalf("GetAuthenticatedUser: %v", err)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(*requests))
	}
	got := (*requests)[0]
	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if got.Header.Get("Authorization") != "token test-token" {
		t.Errorf("Authorization = %q, want %q", got.Header.Get("Authorization"), "token test-token")
	}
	if got.Header.Get("Accept") != githubAccept {
		t.Errorf("Accept = %q, want %q", got.Header.Get("Accept"), githubAccept)
	}
	if got.Header.Get("X-GitHub-Api-Version") != githubAPIVersion {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", got.Header.Get("X-GitHub-Api-Version"), githubAPIVersion)
	}
}

func TestPATClient_GetAuthenticatedUser_CachesLogin(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{"/user": `{"login":"octocat"}`})

	first, err := c.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := c.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != "octocat" || second != "octocat" {
		t.Fatalf("logins = (%q, %q), want both octocat", first, second)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %d, want 1 (second call must hit the cache)", len(*requests))
	}
}

func TestPATClient_IsAuthenticated(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		c, _ := newRecordingPATServer(t, map[string]string{"/user": `{"login":"octocat"}`})
		ok, err := c.IsAuthenticated(context.Background())
		if err != nil {
			t.Fatalf("IsAuthenticated: %v", err)
		}
		if !ok {
			t.Error("authenticated = false, want true")
		}
	})
	t.Run("rejected token reports false without an error", func(t *testing.T) {
		// No /user route → 404 → GetAuthenticatedUser errors, which
		// IsAuthenticated must translate into (false, nil).
		c, _ := newRecordingPATServer(t, nil)
		ok, err := c.IsAuthenticated(context.Background())
		if err != nil {
			t.Fatalf("IsAuthenticated must swallow the auth failure, got %v", err)
		}
		if ok {
			t.Error("authenticated = true, want false")
		}
	})
}

func TestPATClient_HasRepositoryAccess(t *testing.T) {
	t.Run("repo readable", func(t *testing.T) {
		c, requests := newRecordingPATServer(t, map[string]string{
			"/repos/acme/widget": `{"full_name":"acme/widget"}`,
		})
		ok, err := c.HasRepositoryAccess(context.Background(), "acme", "widget")
		if err != nil {
			t.Fatalf("HasRepositoryAccess: %v", err)
		}
		if !ok {
			t.Error("access = false, want true")
		}
		if (*requests)[0].Path != "/repos/acme/widget" {
			t.Errorf("path = %q, want /repos/acme/widget", (*requests)[0].Path)
		}
	})
	t.Run("empty full_name reports no access", func(t *testing.T) {
		c, _ := newRecordingPATServer(t, map[string]string{"/repos/acme/widget": `{}`})
		ok, err := c.HasRepositoryAccess(context.Background(), "acme", "widget")
		if err != nil {
			t.Fatalf("HasRepositoryAccess: %v", err)
		}
		if ok {
			t.Error("access = true, want false when full_name is absent")
		}
	})
	t.Run("404 surfaces a typed API error", func(t *testing.T) {
		c, _ := newRecordingPATServer(t, nil)
		ok, err := c.HasRepositoryAccess(context.Background(), "acme", "widget")
		if ok {
			t.Error("access = true, want false on error")
		}
		var apiErr *GitHubAPIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
			t.Fatalf("err = %v, want *GitHubAPIError with 404", err)
		}
	})
}

func TestPATClient_GetPR_DecodesEveryField(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/42": `{
			"number":42,"title":"Add widgets","html_url":"https://github.com/acme/widget/pull/42",
			"body":"the body","state":"open","draft":true,"mergeable":true,"mergeable_state":"CLEAN",
			"additions":12,"deletions":3,
			"created_at":"2026-01-01T10:00:00Z","updated_at":"2026-01-02T11:00:00Z",
			"merged_at":null,"closed_at":null,"maintainer_can_modify":true,
			"requested_reviewers":[{"login":"alice"}],
			"requested_teams":[{"slug":"platform","name":"Platform Team"}],
			"user":{"login":"bob"},
			"head":{"ref":"feature","sha":"abc123","repo":{"id":77,"name":"widget-fork","full_name":"contributor/widget-fork","clone_url":"https://github.com/contributor/widget-fork.git"}},
			"base":{"ref":"main","repo":{"id":11,"name":"widget","full_name":"acme/widget","default_branch":"trunk"}}
		}`,
	})

	pr, err := c.GetPR(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if (*requests)[0].Path != "/repos/acme/widget/pulls/42" {
		t.Errorf("path = %q, want /repos/acme/widget/pulls/42", (*requests)[0].Path)
	}
	assertPRScalars(t, pr)
	assertPRRepositories(t, pr)
	if len(pr.RequestedReviewers) != 2 {
		t.Fatalf("requested reviewers = %#v, want user + team", pr.RequestedReviewers)
	}
	if pr.RequestedReviewers[0] != (RequestedReviewer{Login: "alice", Type: reviewerTypeUser}) {
		t.Errorf("reviewer[0] = %#v", pr.RequestedReviewers[0])
	}
	if pr.RequestedReviewers[1] != (RequestedReviewer{Login: "platform", Type: reviewerTypeTeam}) {
		t.Errorf("reviewer[1] = %#v", pr.RequestedReviewers[1])
	}
	if pr.MergedAt != nil || pr.ClosedAt != nil {
		t.Errorf("MergedAt = %v, ClosedAt = %v, want both nil", pr.MergedAt, pr.ClosedAt)
	}
}

func assertPRScalars(t *testing.T, pr *PR) {
	t.Helper()
	if pr.Number != 42 || pr.Title != "Add widgets" {
		t.Errorf("number/title = (%d, %q)", pr.Number, pr.Title)
	}
	if pr.HTMLURL != "https://github.com/acme/widget/pull/42" {
		t.Errorf("html_url = %q", pr.HTMLURL)
	}
	if pr.Body != "the body" || pr.State != "open" {
		t.Errorf("body/state = (%q, %q)", pr.Body, pr.State)
	}
	if !pr.Draft || !pr.Mergeable || !pr.MaintainerCanModify {
		t.Errorf("draft=%v mergeable=%v maintainerCanModify=%v, want all true",
			pr.Draft, pr.Mergeable, pr.MaintainerCanModify)
	}
	if pr.MergeableState != "clean" {
		t.Errorf("mergeable_state = %q, want lowercased clean", pr.MergeableState)
	}
	if pr.Additions != 12 || pr.Deletions != 3 {
		t.Errorf("additions/deletions = (%d, %d), want (12, 3)", pr.Additions, pr.Deletions)
	}
	if pr.HeadBranch != "feature" || pr.HeadSHA != "abc123" || pr.BaseBranch != "main" {
		t.Errorf("head/base = (%q, %q, %q)", pr.HeadBranch, pr.HeadSHA, pr.BaseBranch)
	}
	if pr.AuthorLogin != "bob" || pr.RepoOwner != "acme" || pr.RepoName != "widget" {
		t.Errorf("author/repo = (%q, %q, %q)", pr.AuthorLogin, pr.RepoOwner, pr.RepoName)
	}
	if !pr.CreatedAt.Equal(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("created_at = %v", pr.CreatedAt)
	}
	if !pr.UpdatedAt.Equal(time.Date(2026, 1, 2, 11, 0, 0, 0, time.UTC)) {
		t.Errorf("updated_at = %v", pr.UpdatedAt)
	}
}

// assertPRRepositories pins the nested head.repo / base.repo decode, which
// only happens through patPR's custom UnmarshalJSON.
func assertPRRepositories(t *testing.T, pr *PR) {
	t.Helper()
	if pr.HeadRepoID != 77 || pr.HeadRepoOwner != "contributor" || pr.HeadRepoName != "widget-fork" {
		t.Errorf("head repo = (%d, %q, %q), want (77, contributor, widget-fork)",
			pr.HeadRepoID, pr.HeadRepoOwner, pr.HeadRepoName)
	}
	if pr.HeadRepoCloneURL != "https://github.com/contributor/widget-fork.git" {
		t.Errorf("head clone url = %q", pr.HeadRepoCloneURL)
	}
	if pr.BaseRepoID != 11 || pr.BaseRepoOwner != "acme" || pr.BaseRepoName != "widget" {
		t.Errorf("base repo = (%d, %q, %q), want (11, acme, widget)",
			pr.BaseRepoID, pr.BaseRepoOwner, pr.BaseRepoName)
	}
	if pr.BaseDefaultBranch != "trunk" {
		t.Errorf("base default branch = %q, want trunk", pr.BaseDefaultBranch)
	}
}

// TestPATClient_GetPR_DecodesOutcomeFields covers AC-09's REST half:
// changed_files, merged_by, and a non-null auto_merge decode onto PR.
func TestPATClient_GetPR_DecodesOutcomeFields(t *testing.T) {
	c, _ := newRecordingPATServer(t, map[string]string{
		"/repos/kdlbs/kandev/pulls/2554": `{
			"number":2554,"state":"closed","merged_at":"2026-08-12T00:00:00Z",
			"changed_files":8,"merged_by":{"login":"carlosflorencio"},
			"auto_merge":{"enabled_by":{"login":"carlosflorencio"},"merge_method":"squash"},
			"user":{"login":"carlosflorencio"},"head":{"ref":"f"},"base":{"ref":"main"}
		}`,
	})

	pr, err := c.GetPR(context.Background(), "kdlbs", "kandev", 2554)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.ChangedFiles != 8 {
		t.Errorf("ChangedFiles = %d, want 8", pr.ChangedFiles)
	}
	if pr.MergedByLogin != "carlosflorencio" {
		t.Errorf("MergedByLogin = %q, want carlosflorencio", pr.MergedByLogin)
	}
	if !pr.AutoMergeEnabled {
		t.Error("AutoMergeEnabled = false, want true (non-null auto_merge)")
	}
}

// TestPATClient_GetPR_NullMergedByAndAutoMergeDecodeToZeroValues covers the
// "never observed / not armed" side: GitHub reports auto_merge: null once it
// fires (merged PR 2554 in the spec's live sample) or when never armed, and
// merged_by: null on an unmerged PR.
func TestPATClient_GetPR_NullMergedByAndAutoMergeDecodeToZeroValues(t *testing.T) {
	c, _ := newRecordingPATServer(t, map[string]string{
		"/repos/kdlbs/kandev/pulls/2476": `{
			"number":2476,"state":"closed","draft":true,"changed_files":114,
			"merged_by":null,"auto_merge":null,
			"user":{"login":"nova28"},"head":{"ref":"f"},"base":{"ref":"main"}
		}`,
	})

	pr, err := c.GetPR(context.Background(), "kdlbs", "kandev", 2476)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.MergedByLogin != "" {
		t.Errorf("MergedByLogin = %q, want empty", pr.MergedByLogin)
	}
	if pr.AutoMergeEnabled {
		t.Error("AutoMergeEnabled = true, want false (null auto_merge)")
	}
	if pr.ChangedFiles != 114 {
		t.Errorf("ChangedFiles = %d, want 114", pr.ChangedFiles)
	}
}

func TestPATClient_GetPR_MergedStateAndTimestamps(t *testing.T) {
	c, _ := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/pulls/9": `{"number":9,"state":"closed",
			"merged_at":"2026-02-03T09:00:00Z","closed_at":"2026-02-03T09:00:00Z",
			"user":{"login":"bob"},"head":{"ref":"f"},"base":{"ref":"main"}}`,
	})

	pr, err := c.GetPR(context.Background(), "acme", "widget", 9)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.State != prStateMerged {
		t.Errorf("state = %q, want merged (merged_at promotes closed → merged)", pr.State)
	}
	want := time.Date(2026, 2, 3, 9, 0, 0, 0, time.UTC)
	if pr.MergedAt == nil || !pr.MergedAt.Equal(want) {
		t.Errorf("merged_at = %v, want %v", pr.MergedAt, want)
	}
	if pr.ClosedAt == nil || !pr.ClosedAt.Equal(want) {
		t.Errorf("closed_at = %v, want %v", pr.ClosedAt, want)
	}
	// No head.repo / base.repo in the payload: the identity fields stay zero.
	if pr.HeadRepoID != 0 || pr.BaseRepoID != 0 || pr.BaseDefaultBranch != "" {
		t.Errorf("expected zero repo identity, got head=%d base=%d branch=%q",
			pr.HeadRepoID, pr.BaseRepoID, pr.BaseDefaultBranch)
	}
}

func TestPATClient_GetPR_ErrorIsWrappedWithNumber(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	pr, err := c.GetPR(context.Background(), "acme", "widget", 42)
	if pr != nil {
		t.Errorf("pr = %#v, want nil on error", pr)
	}
	if err == nil || !strings.Contains(err.Error(), "get PR #42") {
		t.Fatalf("err = %v, want it to name PR #42", err)
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want an unwrappable *GitHubAPIError with 404", err)
	}
}

func TestPATClient_GetIssue_DecodesEveryField(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/issues/5": `{
			"number":5,"title":"Broken login","html_url":"https://github.com/acme/widget/issues/5",
			"body":"steps to repro","state":"CLOSED",
			"created_at":"2026-03-01T08:00:00Z","updated_at":"2026-03-02T08:00:00Z",
			"closed_at":"2026-03-03T08:00:00Z",
			"user":{"login":"carol"},
			"labels":[{"name":"bug"},{"name":"p1"}],
			"assignees":[{"login":"dave"}]
		}`,
	})

	issue, err := c.GetIssue(context.Background(), "acme", "widget", 5)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if (*requests)[0].Path != "/repos/acme/widget/issues/5" {
		t.Errorf("path = %q, want /repos/acme/widget/issues/5", (*requests)[0].Path)
	}
	if issue.Number != 5 || issue.Title != "Broken login" || issue.Body != "steps to repro" {
		t.Errorf("issue scalars = %#v", issue)
	}
	if issue.URL != "https://github.com/acme/widget/issues/5" || issue.HTMLURL != issue.URL {
		t.Errorf("URL/HTMLURL = (%q, %q), want both the html_url", issue.URL, issue.HTMLURL)
	}
	if issue.State != "closed" {
		t.Errorf("state = %q, want lowercased closed", issue.State)
	}
	if issue.AuthorLogin != "carol" || issue.RepoOwner != "acme" || issue.RepoName != "widget" {
		t.Errorf("author/repo = (%q, %q, %q)", issue.AuthorLogin, issue.RepoOwner, issue.RepoName)
	}
	if strings.Join(issue.Labels, ",") != "bug,p1" {
		t.Errorf("labels = %v, want [bug p1]", issue.Labels)
	}
	if strings.Join(issue.Assignees, ",") != "dave" {
		t.Errorf("assignees = %v, want [dave]", issue.Assignees)
	}
	if !issue.CreatedAt.Equal(time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("created_at = %v", issue.CreatedAt)
	}
	if !issue.UpdatedAt.Equal(time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("updated_at = %v", issue.UpdatedAt)
	}
	wantClosed := time.Date(2026, 3, 3, 8, 0, 0, 0, time.UTC)
	if issue.ClosedAt == nil || !issue.ClosedAt.Equal(wantClosed) {
		t.Errorf("closed_at = %v, want %v", issue.ClosedAt, wantClosed)
	}
}

func TestPATClient_GetIssue_OpenIssueHasNilClosedAt(t *testing.T) {
	c, _ := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/issues/6": `{"number":6,"state":"open","closed_at":null,"user":{"login":"carol"}}`,
	})
	issue, err := c.GetIssue(context.Background(), "acme", "widget", 6)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.ClosedAt != nil {
		t.Errorf("closed_at = %v, want nil for an open issue", issue.ClosedAt)
	}
	if len(issue.Labels) != 0 || len(issue.Assignees) != 0 {
		t.Errorf("labels/assignees = (%v, %v), want empty", issue.Labels, issue.Assignees)
	}
}

func TestPATClient_GetIssue_ErrorIsWrappedWithNumber(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	if _, err := c.GetIssue(context.Background(), "acme", "widget", 6); err == nil ||
		!strings.Contains(err.Error(), "get issue #6") {
		t.Fatalf("err = %v, want it to name issue #6", err)
	}
}

func TestPATClient_GetIssueState(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/issues/7": `{"state":"closed","number":7}`,
	})
	state, err := c.GetIssueState(context.Background(), "acme", "widget", 7)
	if err != nil {
		t.Fatalf("GetIssueState: %v", err)
	}
	if state != "closed" {
		t.Errorf("state = %q, want closed", state)
	}
	if (*requests)[0].Path != "/repos/acme/widget/issues/7" {
		t.Errorf("path = %q", (*requests)[0].Path)
	}
}

func TestPATClient_GetIssueState_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	state, err := c.GetIssueState(context.Background(), "acme", "widget", 7)
	if state != "" {
		t.Errorf("state = %q, want empty on error", state)
	}
	if err == nil || !strings.Contains(err.Error(), "get issue state") {
		t.Fatalf("err = %v, want a wrapped get-issue-state error", err)
	}
}

func TestPATClient_ListAuthoredPRs_KeepsOnlyTheAuthenticatedUsersPRs(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/user": `{"login":"octocat"}`,
		"/repos/acme/widget/pulls": `[
			{"number":1,"title":"mine","state":"open","user":{"login":"octocat"},"head":{"ref":"a"},"base":{"ref":"main"}},
			{"number":2,"title":"theirs","state":"open","user":{"login":"someone-else"},"head":{"ref":"b"},"base":{"ref":"main"}},
			{"number":3,"title":"mine too","state":"open","user":{"login":"octocat"},"head":{"ref":"c"},"base":{"ref":"main"}}
		]`,
	})

	prs, err := c.ListAuthoredPRs(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ListAuthoredPRs: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("prs = %d, want 2 (the other author must be filtered out)", len(prs))
	}
	if prs[0].Number != 1 || prs[1].Number != 3 {
		t.Errorf("numbers = (%d, %d), want (1, 3)", prs[0].Number, prs[1].Number)
	}
	if prs[0].RepoOwner != "acme" || prs[0].RepoName != "widget" {
		t.Errorf("repo = (%q, %q), want acme/widget", prs[0].RepoOwner, prs[0].RepoName)
	}
	listQuery := (*requests)[1].Query
	if !strings.Contains(listQuery, "state=open") || !strings.Contains(listQuery, "per_page=100") {
		t.Errorf("query = %q, want state=open and per_page=100", listQuery)
	}
}

func TestPATClient_ListAuthoredPRs_PropagatesAuthFailure(t *testing.T) {
	c, requests := newRecordingPATServer(t, nil)
	prs, err := c.ListAuthoredPRs(context.Background(), "acme", "widget")
	if err == nil {
		t.Fatal("expected an error when /user fails")
	}
	if prs != nil {
		t.Errorf("prs = %v, want nil", prs)
	}
	if len(*requests) != 1 {
		t.Errorf("requests = %d, want 1 — the PR list must not run after auth fails", len(*requests))
	}
}

func TestPATClient_ListReviewRequestedPRs_BuildsScopedQuery(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/search/issues": `{"items":[{"id":501,"node_id":"PR_a","number":3,"title":"Review me",
			"html_url":"https://github.com/acme/web/pull/3","state":"closed","draft":true,
			"created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-02T00:00:00Z",
			"repository_url":"https://api.github.com/repos/acme/web",
			"user":{"login":"erin"},"pull_request":{"merged_at":"2026-04-03T00:00:00Z"}}]}`,
	})

	prs, err := c.ListReviewRequestedPRs(context.Background(), ReviewScopeUser, "repo:acme/web", "")
	if err != nil {
		t.Fatalf("ListReviewRequestedPRs: %v", err)
	}
	query := (*requests)[0].Query
	for _, want := range []string{"user-review-requested", "per_page=50", "repo%3Aacme%2Fweb"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, want it to contain %q", query, want)
		}
	}
	if len(prs) != 1 {
		t.Fatalf("prs = %d, want 1", len(prs))
	}
	pr := prs[0]
	if pr.ID != 501 || pr.NodeID != "PR_a" || pr.Number != 3 || pr.Title != "Review me" {
		t.Errorf("identity = %#v", pr)
	}
	if pr.State != prStateMerged {
		t.Errorf("state = %q, want merged — a non-empty pull_request.merged_at promotes closed", pr.State)
	}
	if pr.AuthorLogin != "erin" || !pr.Draft {
		t.Errorf("author/draft = (%q, %v)", pr.AuthorLogin, pr.Draft)
	}
	if pr.RepoOwner != "acme" || pr.RepoName != "web" {
		t.Errorf("repo = (%q, %q), want acme/web parsed from repository_url", pr.RepoOwner, pr.RepoName)
	}
	if pr.MergedAt == nil || !pr.MergedAt.Equal(time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("merged_at = %v", pr.MergedAt)
	}
}

func TestPATClient_ListReviewRequestedPRs_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	if _, err := c.ListReviewRequestedPRs(context.Background(), "", "", ""); err == nil ||
		!strings.Contains(err.Error(), "search review-requested") {
		t.Fatalf("err = %v, want a wrapped search error", err)
	}
}

func TestPATClient_SearchPRs_UsesFirstPageOfFifty(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/search/issues": `{"total_count":2,"items":[
			{"id":1,"number":1,"title":"one","state":"open","repository_url":"https://api.github.com/repos/o/r","pull_request":{}},
			{"id":2,"number":2,"title":"two","state":"open","repository_url":"https://api.github.com/repos/o/r","pull_request":{}}
		]}`,
	})

	prs, err := c.SearchPRs(context.Background(), "is:open", "")
	if err != nil {
		t.Fatalf("SearchPRs: %v", err)
	}
	if len(prs) != 2 || prs[0].Number != 1 || prs[1].Number != 2 {
		t.Fatalf("prs = %#v", prs)
	}
	query := (*requests)[0].Query
	if !strings.Contains(query, "per_page=50") || !strings.Contains(query, "page=1") {
		t.Errorf("query = %q, want per_page=50 and page=1", query)
	}
	if !strings.Contains(query, "type%3Apr") {
		t.Errorf("query = %q, want the injected type:pr qualifier", query)
	}
}

func TestPATClient_SearchPRsPaged_ClampsPageAndPerPage(t *testing.T) {
	cases := []struct {
		name                  string
		page, perPage         int
		wantPage, wantPerPage int
	}{
		{"zero page floors to 1", 0, 25, 1, 25},
		{"negative page floors to 1", -3, 25, 1, 25},
		{"zero per_page defaults to 50", 2, 0, 2, 50},
		{"per_page above cap clamps to 100", 2, 500, 2, 100},
		{"in-range values pass through", 3, 30, 3, 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := newRecordingPATServer(t, map[string]string{
				"/search/issues": `{"total_count":7,"items":[]}`,
			})
			page, err := c.SearchPRsPaged(context.Background(), "", "", tc.page, tc.perPage)
			if err != nil {
				t.Fatalf("SearchPRsPaged: %v", err)
			}
			if page.Page != tc.wantPage || page.PerPage != tc.wantPerPage {
				t.Errorf("page/per_page = (%d, %d), want (%d, %d)",
					page.Page, page.PerPage, tc.wantPage, tc.wantPerPage)
			}
			if page.TotalCount != 7 {
				t.Errorf("total_count = %d, want 7", page.TotalCount)
			}
			if len(page.PRs) != 0 {
				t.Errorf("prs = %#v, want empty", page.PRs)
			}
			gotQuery := (*requests)[0].Query
			for _, want := range []string{
				"page=" + strconv.Itoa(tc.wantPage), "per_page=" + strconv.Itoa(tc.wantPerPage),
			} {
				if !strings.Contains(gotQuery, want) {
					t.Errorf("query = %q, want %q", gotQuery, want)
				}
			}
		})
	}
}

func TestPATClient_ListIssuesPaged_DropsPullRequestsFromSearchResults(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/search/issues": `{"total_count":3,"items":[
			{"id":10,"node_id":"I_a","number":10,"title":"a real issue","body":"details",
			 "html_url":"https://github.com/o/r/issues/10","state":"open",
			 "created_at":"2026-05-01T00:00:00Z","updated_at":"2026-05-02T00:00:00Z",
			 "repository_url":"https://api.github.com/repos/o/r","user":{"login":"frank"},
			 "labels":[{"name":"bug"}],"assignees":[{"login":"grace"}]},
			{"id":11,"number":11,"title":"a PR in disguise","state":"open",
			 "repository_url":"https://api.github.com/repos/o/r","pull_request":{"url":"https://api/pulls/11"}}
		]}`,
	})

	page, err := c.ListIssuesPaged(context.Background(), "", "", 1, 20)
	if err != nil {
		t.Fatalf("ListIssuesPaged: %v", err)
	}
	if len(page.Issues) != 1 {
		t.Fatalf("issues = %d, want 1 — the pull_request item must be dropped", len(page.Issues))
	}
	got := page.Issues[0]
	if got.ID != 10 || got.NodeID != "I_a" || got.Number != 10 {
		t.Errorf("identity = %#v", got)
	}
	if got.Title != "a real issue" || got.Body != "details" || got.State != "open" {
		t.Errorf("scalars = %#v", got)
	}
	if got.RepoOwner != "o" || got.RepoName != "r" || got.AuthorLogin != "frank" {
		t.Errorf("repo/author = (%q, %q, %q)", got.RepoOwner, got.RepoName, got.AuthorLogin)
	}
	if strings.Join(got.Labels, ",") != "bug" || strings.Join(got.Assignees, ",") != "grace" {
		t.Errorf("labels/assignees = (%v, %v)", got.Labels, got.Assignees)
	}
	// TotalCount is reported verbatim by GitHub — it is NOT the filtered length.
	if page.TotalCount != 3 || page.Page != 1 || page.PerPage != 20 {
		t.Errorf("page meta = (%d, %d, %d), want (3, 1, 20)", page.TotalCount, page.Page, page.PerPage)
	}
	if !strings.Contains((*requests)[0].Query, "type%3Aissue") {
		t.Errorf("query = %q, want the injected type:issue qualifier", (*requests)[0].Query)
	}
}

func TestPATClient_ListIssues_DelegatesToFirstPage(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/search/issues": `{"total_count":1,"items":[{"id":1,"number":1,"title":"x",
			"repository_url":"https://api.github.com/repos/o/r"}]}`,
	})
	issues, err := c.ListIssues(context.Background(), "", "is:open label:bug")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 || issues[0].Number != 1 {
		t.Fatalf("issues = %#v", issues)
	}
	query := (*requests)[0].Query
	if !strings.Contains(query, "per_page=50") || !strings.Contains(query, "page=1") {
		t.Errorf("query = %q, want the default first page of 50", query)
	}
}

func TestPATClient_ListIssuesPaged_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	if _, err := c.ListIssuesPaged(context.Background(), "", "", 1, 10); err == nil ||
		!strings.Contains(err.Error(), "search issues") {
		t.Fatalf("err = %v, want a wrapped search-issues error", err)
	}
}

func TestPATClient_ListUserOrgs(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/user/orgs": `[{"login":"acme","avatar_url":"https://avatars/acme"},{"login":"other","avatar_url":""}]`,
	})
	orgs, err := c.ListUserOrgs(context.Background())
	if err != nil {
		t.Fatalf("ListUserOrgs: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("orgs = %d, want 2", len(orgs))
	}
	if orgs[0] != (GitHubOrg{Login: "acme", AvatarURL: "https://avatars/acme"}) {
		t.Errorf("orgs[0] = %#v", orgs[0])
	}
	if orgs[1] != (GitHubOrg{Login: "other"}) {
		t.Errorf("orgs[1] = %#v", orgs[1])
	}
	if !strings.Contains((*requests)[0].Query, "per_page=100") {
		t.Errorf("query = %q, want per_page=100", (*requests)[0].Query)
	}
}

func TestPATClient_ListUserOrgs_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	orgs, err := c.ListUserOrgs(context.Background())
	if orgs != nil {
		t.Errorf("orgs = %v, want nil", orgs)
	}
	if err == nil || !strings.Contains(err.Error(), "list user orgs") {
		t.Fatalf("err = %v, want a wrapped list-user-orgs error", err)
	}
}

func TestPATClient_SearchOrgRepos(t *testing.T) {
	cases := []struct {
		name  string
		query string
		limit int
		wantQ string
		wantP string
	}{
		{"no free-text query", "", 30, "org:acme", "30"},
		{"with free-text query", "widget", 30, "org:acme widget", "30"},
		{"limit clamps to the cap", "", 900, "org:acme", "100"},
		{"zero limit takes the default", "", 0, "org:acme", "20"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, requests := newRecordingPATServer(t, map[string]string{
				"/search/repositories": `{"items":[{"full_name":"acme/widget","owner":{"login":"acme"},
					"name":"widget","private":true,"default_branch":"main","description":"w",
					"pushed_at":"2026-06-01T00:00:00Z"}]}`,
			})
			repos, err := c.SearchOrgRepos(context.Background(), "acme", tc.query, tc.limit)
			if err != nil {
				t.Fatalf("SearchOrgRepos: %v", err)
			}
			if len(repos) != 1 {
				t.Fatalf("repos = %d, want 1", len(repos))
			}
			got := repos[0]
			if got.FullName != "acme/widget" || got.Owner != "acme" || got.Name != "widget" {
				t.Errorf("identity = %#v", got)
			}
			if !got.Private || got.DefaultBranch != "main" || got.Description != "w" {
				t.Errorf("attributes = %#v", got)
			}
			if got.PushedAt == nil || !got.PushedAt.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
				t.Errorf("pushed_at = %v", got.PushedAt)
			}
			values := parseQueryValues(t, (*requests)[0].Query)
			if values.Get("q") != tc.wantQ {
				t.Errorf("q = %q, want %q", values.Get("q"), tc.wantQ)
			}
			if values.Get("per_page") != tc.wantP {
				t.Errorf("per_page = %q, want %q", values.Get("per_page"), tc.wantP)
			}
		})
	}
}

func TestPATClient_SearchOrgRepos_Error(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	repos, err := c.SearchOrgRepos(context.Background(), "acme", "", 10)
	if repos != nil {
		t.Errorf("repos = %v, want nil", repos)
	}
	if err == nil || !strings.Contains(err.Error(), "search org repos") {
		t.Fatalf("err = %v, want a wrapped search-org-repos error", err)
	}
}
