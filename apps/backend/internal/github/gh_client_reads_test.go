package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGHClient_GetIssue(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "issue view",
		Stdout: `{"number":5,"title":"Broken login","url":"https://github.com/acme/widget/issues/5",
			"state":"CLOSED","body":"steps","createdAt":"2026-03-01T08:00:00Z",
			"updatedAt":"2026-03-02T08:00:00Z","closedAt":"2026-03-03T08:00:00Z",
			"author":{"login":"carol"},"labels":[{"name":"bug"},{"name":"p1"}],
			"assignees":[{"login":"dave"}]}`,
	})

	issue, err := NewGHClient().GetIssue(context.Background(), "acme", "widget", 5)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"issue", "view", "5", "--repo", "acme/widget",
		"--json", "number,title,url,state,body,author,labels,assignees,createdAt,updatedAt,closedAt",
	})
	if issue.Number != 5 || issue.Title != "Broken login" || issue.Body != "steps" {
		t.Errorf("scalars = %#v", issue)
	}
	if issue.URL != "https://github.com/acme/widget/issues/5" || issue.HTMLURL != issue.URL {
		t.Errorf("URL/HTMLURL = (%q, %q)", issue.URL, issue.HTMLURL)
	}
	if issue.State != "closed" {
		t.Errorf("state = %q, want lowercased closed", issue.State)
	}
	if issue.AuthorLogin != "carol" || issue.RepoOwner != "acme" || issue.RepoName != "widget" {
		t.Errorf("author/repo = (%q, %q, %q)", issue.AuthorLogin, issue.RepoOwner, issue.RepoName)
	}
	if strings.Join(issue.Labels, ",") != "bug,p1" || strings.Join(issue.Assignees, ",") != "dave" {
		t.Errorf("labels/assignees = (%v, %v)", issue.Labels, issue.Assignees)
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

// The gh CLI emits "" (not null) for an open issue's closedAt.
func TestGHClient_GetIssue_EmptyClosedAtDecodesToNil(t *testing.T) {
	newFakeGH(t, ghResponse{
		Prefix: "issue view",
		Stdout: `{"number":6,"state":"OPEN","closedAt":"","author":{"login":"carol"}}`,
	})
	issue, err := NewGHClient().GetIssue(context.Background(), "acme", "widget", 6)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.ClosedAt != nil {
		t.Errorf("closed_at = %v, want nil", issue.ClosedAt)
	}
}

func TestGHClient_GetIssue_NotFoundBecomesTypedError(t *testing.T) {
	newFakeGH(t, ghResponse{
		Prefix: "issue view",
		Stderr: "gh: HTTP 404: Not Found",
		Exit:   1,
	})
	issue, err := NewGHClient().GetIssue(context.Background(), "acme", "widget", 404)
	if issue != nil {
		t.Errorf("issue = %#v, want nil", issue)
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want a *GitHubAPIError with 404", err)
	}
	if apiErr.Endpoint != "/repos/acme/widget/issues/404" {
		t.Errorf("endpoint = %q", apiErr.Endpoint)
	}
}

func TestGHClient_GetIssue_OtherFailureIsWrapped(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "issue view", Stderr: "HTTP 500: server error", Exit: 1})
	_, err := NewGHClient().GetIssue(context.Background(), "acme", "widget", 6)
	if err == nil || !strings.Contains(err.Error(), "get issue #6") {
		t.Fatalf("err = %v, want a wrapped get-issue error", err)
	}
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) {
		t.Errorf("a 500 must not be promoted to *GitHubAPIError, got %#v", apiErr)
	}
}

func TestGHClient_GetRepository_NotFoundBecomesTypedError(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api /repos/acme/widget", Stderr: "gh: 404 Not Found", Exit: 1})
	repository, err := NewGHClient().GetRepository(context.Background(), "acme", "widget")
	if repository != nil {
		t.Errorf("repository = %#v, want nil", repository)
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want a *GitHubAPIError with 404", err)
	}
	if apiErr.Endpoint != "/repos/acme/widget" {
		t.Errorf("endpoint = %q", apiErr.Endpoint)
	}
}

func TestGHClient_ListRepositoryForksUsesForkNetwork(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/acme/widget/forks?per_page=100",
		Stdout: `[[{"id":200,"full_name":"alice/widget-renamed","name":"widget-renamed","owner":{"login":"alice"},"fork":true,"parent":{"id":100,"full_name":"acme/widget"},"permissions":{"push":true}}]]`,
	})
	forks, err := NewGHClient().ListRepositoryForks(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ListRepositoryForks: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "repos/acme/widget/forks?per_page=100", "--paginate", "--slurp",
	})
	if len(forks) != 1 || forks[0].FullName != "alice/widget-renamed" || forks[0].ParentID != 100 {
		t.Fatalf("forks = %#v", forks)
	}
}

func TestGHClient_GetPR_NotFoundBecomesTypedError(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "pr view", Stderr: "gh: HTTP 404: Not Found", Exit: 1})
	pr, err := NewGHClient().GetPR(context.Background(), "acme", "widget", 7)
	if pr != nil {
		t.Errorf("pr = %#v, want nil", pr)
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want a *GitHubAPIError with 404", err)
	}
	if apiErr.Endpoint != "/repos/acme/widget/pulls/7" {
		t.Errorf("endpoint = %q", apiErr.Endpoint)
	}
}

// TestGHClient_GetPR_RequestsOutcomeFields covers AC-09: the --json field
// list gains changedFiles, mergedBy, autoMergeRequest, and never requests
// closedBy (absent from the gh CLI's PR field set). The decoded PR carries
// the three new values.
func TestGHClient_GetPR_RequestsOutcomeFields(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "pr view",
		Stdout: `{"number":2554,"title":"fix","url":"https://github.com/kdlbs/kandev/pull/2554",
			"state":"MERGED","headRefName":"h","headRefOid":"abc","baseRefName":"main",
			"isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN",
			"additions":10,"deletions":2,"changedFiles":8,
			"author":{"login":"alice"},"mergedBy":{"login":"carlosflorencio"},
			"autoMergeRequest":{"enabledAt":"2026-08-12T00:00:00Z"},
			"createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-02T00:00:00Z",
			"mergedAt":"2026-08-12T00:00:00Z","closedAt":"2026-08-12T00:00:00Z"}`,
	})

	pr, err := NewGHClient().GetPR(context.Background(), "kdlbs", "kandev", 2554)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}

	argv := calls(t)[0]
	jsonArgIdx := -1
	for i, a := range argv {
		if a == "--json" {
			jsonArgIdx = i + 1
			break
		}
	}
	if jsonArgIdx < 0 || jsonArgIdx >= len(argv) {
		t.Fatalf("no --json argument found in %v", argv)
	}
	fieldList := argv[jsonArgIdx]
	for _, want := range []string{"changedFiles", "mergedBy", "autoMergeRequest"} {
		if !strings.Contains(fieldList, want) {
			t.Errorf("--json field list missing %q: %s", want, fieldList)
		}
	}
	if strings.Contains(fieldList, "closedBy") {
		t.Errorf("--json field list must not request closedBy (not in gh CLI's PR field set): %s", fieldList)
	}

	if pr.ChangedFiles != 8 {
		t.Errorf("ChangedFiles = %d, want 8", pr.ChangedFiles)
	}
	if pr.MergedByLogin != "carlosflorencio" {
		t.Errorf("MergedByLogin = %q, want carlosflorencio", pr.MergedByLogin)
	}
	if !pr.AutoMergeEnabled {
		t.Error("AutoMergeEnabled = false, want true (non-null autoMergeRequest)")
	}
}

// TestGHClient_GetPR_NoMergerOrAutoMergeDecodesToZeroValues covers the
// "never observed / not armed" side: a null mergedBy and a null
// autoMergeRequest must decode to an empty login and AutoMergeEnabled=false,
// never a placeholder value.
func TestGHClient_GetPR_NoMergerOrAutoMergeDecodesToZeroValues(t *testing.T) {
	newFakeGH(t, ghResponse{
		Prefix: "pr view",
		Stdout: `{"number":2476,"title":"wip","url":"https://github.com/kdlbs/kandev/pull/2476",
			"state":"CLOSED","isDraft":true,"changedFiles":114,
			"author":{"login":"alice"},"mergedBy":null,"autoMergeRequest":null,
			"createdAt":"2026-08-01T00:00:00Z","updatedAt":"2026-08-02T00:00:00Z","closedAt":"2026-08-05T00:00:00Z"}`,
	})

	pr, err := NewGHClient().GetPR(context.Background(), "kdlbs", "kandev", 2476)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.MergedByLogin != "" {
		t.Errorf("MergedByLogin = %q, want empty", pr.MergedByLogin)
	}
	if pr.AutoMergeEnabled {
		t.Error("AutoMergeEnabled = true, want false (null autoMergeRequest)")
	}
	if pr.ChangedFiles != 114 {
		t.Errorf("ChangedFiles = %d, want 114", pr.ChangedFiles)
	}
}

func TestGHClient_GetPR_UnparseableOutput(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "pr view", Stdout: "not json"})
	if _, err := NewGHClient().GetPR(context.Background(), "acme", "widget", 7); err == nil ||
		!strings.Contains(err.Error(), "parse PR response") {
		t.Fatalf("err = %v, want a parse failure", err)
	}
}

func TestGHClient_FindPRByBranch(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "pr list",
		Stdout: `[{"number":12,"title":"fork PR","url":"https://github.com/acme/widget/pull/12",
			"state":"OPEN","headRefName":"feature","headRefOid":"abc123","baseRefName":"main",
			"author":{"login":"alice"},"isDraft":false,"mergeable":"MERGEABLE",
			"mergeStateStatus":"CLEAN","additions":3,"deletions":1,
			"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}]`,
	})

	pr, err := NewGHClient().FindPRByBranch(context.Background(), "acme", "widget", "feature")
	if err != nil {
		t.Fatalf("FindPRByBranch: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"pr", "list", "--repo", "acme/widget", "--head", "feature", "--state", "open",
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft," +
			"mergeable,mergeStateStatus,additions,deletions,createdAt,updatedAt,headRepository,headRepositoryOwner",
		"--limit", "1",
	})
	if pr == nil {
		t.Fatal("pr = nil, want the single listed PR")
	}
	if pr.Number != 12 || pr.HeadBranch != "feature" || pr.HeadSHA != "abc123" {
		t.Errorf("pr = %#v", pr)
	}
	if pr.BaseBranch != "main" || pr.AuthorLogin != "alice" || pr.State != "open" {
		t.Errorf("pr = %#v", pr)
	}
	if !pr.Mergeable || pr.MergeableState != "clean" {
		t.Errorf("mergeable = (%v, %q)", pr.Mergeable, pr.MergeableState)
	}
	if pr.RepoOwner != "acme" || pr.RepoName != "widget" {
		t.Errorf("repo = (%q, %q)", pr.RepoOwner, pr.RepoName)
	}
}

func TestGHClient_FindPRByHead_UsesExactSourceRepository(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "pr list",
		Stdout: `[{
			"number":12,"title":"fork PR","url":"https://github.com/kdlbs/kandev/pull/12",
			"state":"OPEN","headRefName":"feature","headRefOid":"abc123","baseRefName":"main",
			"author":{"login":"alice"},"isDraft":false,"mergeable":"MERGEABLE",
			"mergeStateStatus":"CLEAN","headRepository":{"id":"200","name":"kandev","nameWithOwner":"alice/kandev"},
			"headRepositoryOwner":{"login":"alice"}
		}]`,
	})

	pr, err := NewGHClient().FindPRByHead(context.Background(), "kdlbs", "kandev", "alice", "kandev", "feature")
	if err != nil {
		t.Fatalf("FindPRByHead: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"pr", "list", "--repo", "kdlbs/kandev", "--head", "feature", "--state", "open",
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft," +
			"mergeable,mergeStateStatus,additions,deletions,createdAt,updatedAt,headRepository,headRepositoryOwner",
		"--limit", "100",
	})
	if pr == nil || pr.HeadRepoOwner != "alice" || pr.HeadRepoName != "kandev" {
		t.Fatalf("PR = %#v, want source alice/kandev", pr)
	}
}

func TestGHClient_FindPRByHead_ScansAllMatchingBranches(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "pr list",
		Stdout: `[
			{"number":11,"title":"other fork","url":"https://github.com/kdlbs/kandev/pull/11",
			"state":"OPEN","headRefName":"feature","headRefOid":"other","baseRefName":"main",
			"headRepository":{"name":"other-kandev","nameWithOwner":"alice/other-kandev"},
			"headRepositoryOwner":{"login":"alice"}},
			{"number":12,"title":"requested fork","url":"https://github.com/kdlbs/kandev/pull/12",
			"state":"OPEN","headRefName":"feature","headRefOid":"requested","baseRefName":"main",
			"headRepository":{"name":"kandev","nameWithOwner":"alice/kandev"},
			"headRepositoryOwner":{"login":"alice"}}
		]`,
	})

	pr, err := NewGHClient().FindPRByHead(context.Background(), "kdlbs", "kandev", "alice", "kandev", "feature")
	if err != nil {
		t.Fatalf("FindPRByHead: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"pr", "list", "--repo", "kdlbs/kandev", "--head", "feature", "--state", "open",
		"--json", "number,title,url,state,headRefName,headRefOid,baseRefName,author,isDraft," +
			"mergeable,mergeStateStatus,additions,deletions,createdAt,updatedAt,headRepository,headRepositoryOwner",
		"--limit", "100",
	})
	if pr == nil || pr.Number != 12 || pr.HeadRepoOwner != "alice" {
		t.Fatalf("PR = %#v, want the requested fork PR", pr)
	}
}

func TestGHClient_FindPRByBranch_NoMatchReturnsNilWithoutError(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "pr list", Stdout: `[]`})
	pr, err := NewGHClient().FindPRByBranch(context.Background(), "acme", "widget", "feature")
	if err != nil {
		t.Fatalf("FindPRByBranch: %v", err)
	}
	if pr != nil {
		t.Errorf("pr = %#v, want nil for an empty list", pr)
	}
}

func TestGHClient_FindPRByBranch_Errors(t *testing.T) {
	t.Run("command failure names the branch", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "pr list", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().FindPRByBranch(context.Background(), "acme", "widget", "feature")
		if err == nil || !strings.Contains(err.Error(), `find PR by branch "feature"`) {
			t.Fatalf("err = %v, want the branch named in the wrap", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "pr list", Stdout: "{"})
		_, err := NewGHClient().FindPRByBranch(context.Background(), "acme", "widget", "feature")
		if err == nil || !strings.Contains(err.Error(), "parse PR list") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_ListAuthoredPRs(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "pr list",
		Stdout: `[{"number":1,"title":"first","state":"OPEN","headRefName":"a","baseRefName":"main",
			 "author":{"login":"me"}},
			{"number":2,"title":"second","state":"OPEN","headRefName":"b","baseRefName":"main",
			 "author":{"login":"me"}}]`,
	})

	prs, err := NewGHClient().ListAuthoredPRs(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ListAuthoredPRs: %v", err)
	}
	argv := calls(t)[0]
	// The @me author filter is what scopes this to the signed-in account;
	// without it the call silently lists the whole repo.
	if !containsPair(argv, "--author", "@me") {
		t.Errorf("argv = %q, want --author @me", argv)
	}
	if !containsPair(argv, "--state", "open") || !containsPair(argv, "--repo", "acme/widget") {
		t.Errorf("argv = %q, want --state open and --repo acme/widget", argv)
	}
	if len(prs) != 2 || prs[0].Number != 1 || prs[1].Number != 2 {
		t.Fatalf("prs = %#v", prs)
	}
	if prs[0].RepoOwner != "acme" || prs[0].RepoName != "widget" {
		t.Errorf("repo = (%q, %q)", prs[0].RepoOwner, prs[0].RepoName)
	}
}

func TestGHClient_ListAuthoredPRs_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "pr list", Stderr: "boom", Exit: 1})
		prs, err := NewGHClient().ListAuthoredPRs(context.Background(), "acme", "widget")
		if prs != nil {
			t.Errorf("prs = %#v, want nil", prs)
		}
		if err == nil || !strings.Contains(err.Error(), "list authored PRs") {
			t.Fatalf("err = %v, want a wrapped list-authored error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "pr list", Stdout: "["})
		_, err := NewGHClient().ListAuthoredPRs(context.Background(), "acme", "widget")
		if err == nil || !strings.Contains(err.Error(), "parse PR list") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_ListReviewRequestedPRs(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api search/issues",
		Stdout: `[{"id":501,"node_id":"PR_a","number":3,"title":"Review me",
			"html_url":"https://github.com/acme/web/pull/3","state":"closed","draft":true,
			"created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-02T00:00:00Z",
			"repository_url":"https://api.github.com/repos/acme/web","user":{"login":"erin"},
			"pull_request":{"merged_at":"2026-04-03T00:00:00Z"}}]`,
	})

	prs, err := NewGHClient().ListReviewRequestedPRs(context.Background(), ReviewScopeUser, "repo:acme/web", "")
	if err != nil {
		t.Fatalf("ListReviewRequestedPRs: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "search/issues", "-X", "GET",
		"-f", "q=type:pr state:open user-review-requested:@me -is:draft repo:acme/web",
		"-f", "per_page=50", "--jq", ".items",
	})
	if len(prs) != 1 {
		t.Fatalf("prs = %#v, want one", prs)
	}
	pr := prs[0]
	if pr.ID != 501 || pr.NodeID != "PR_a" || pr.Number != 3 || pr.Title != "Review me" {
		t.Errorf("identity = %#v", pr)
	}
	if pr.State != prStateMerged {
		t.Errorf("state = %q, want merged — merged_at promotes a closed search hit", pr.State)
	}
	if pr.RepoOwner != "acme" || pr.RepoName != "web" || pr.AuthorLogin != "erin" || !pr.Draft {
		t.Errorf("pr = %#v", pr)
	}
}

func TestGHClient_ListReviewRequestedPRs_CustomQueryIsUsedVerbatim(t *testing.T) {
	calls := newFakeGH(t, ghResponse{Prefix: "api search/issues", Stdout: `[]`})
	if _, err := NewGHClient().ListReviewRequestedPRs(
		context.Background(), ReviewScopeUser, "ignored", "is:pr author:me",
	); err != nil {
		t.Fatalf("ListReviewRequestedPRs: %v", err)
	}
	if !containsPair(calls(t)[0], "-f", "q=is:pr author:me") {
		t.Errorf("argv = %q, want the custom query passed through untouched", calls(t)[0])
	}
}

func TestGHClient_ListReviewRequestedPRs_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api search/issues", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().ListReviewRequestedPRs(context.Background(), "", "", "")
		if err == nil || !strings.Contains(err.Error(), "list review-requested PRs") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api search/issues", Stdout: "nope"})
		_, err := NewGHClient().ListReviewRequestedPRs(context.Background(), "", "", "")
		if err == nil || !strings.Contains(err.Error(), "parse search results") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_SearchPRsPaged(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api search/issues",
		Stdout: `{"total_count":42,"items":[{"id":1,"number":1,"title":"one","state":"open",
			"repository_url":"https://api.github.com/repos/o/r","pull_request":{}}]}`,
	})

	page, err := NewGHClient().SearchPRsPaged(context.Background(), "is:open", "", 2, 25)
	if err != nil {
		t.Fatalf("SearchPRsPaged: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "search/issues", "-X", "GET",
		"-f", "q=type:pr is:open", "-f", "per_page=25", "-f", "page=2",
	})
	if page.TotalCount != 42 || page.Page != 2 || page.PerPage != 25 {
		t.Errorf("page meta = (%d, %d, %d), want (42, 2, 25)", page.TotalCount, page.Page, page.PerPage)
	}
	if len(page.PRs) != 1 || page.PRs[0].Number != 1 {
		t.Errorf("prs = %#v", page.PRs)
	}
}

func TestGHClient_SearchPRs_UsesTheDefaultFirstPage(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api search/issues",
		Stdout: `{"total_count":0,"items":[]}`,
	})
	prs, err := NewGHClient().SearchPRs(context.Background(), "", "")
	if err != nil {
		t.Fatalf("SearchPRs: %v", err)
	}
	if len(prs) != 0 {
		t.Errorf("prs = %#v, want empty", prs)
	}
	argv := calls(t)[0]
	if !containsPair(argv, "-f", "per_page=50") || !containsPair(argv, "-f", "page=1") {
		t.Errorf("argv = %q, want the default page 1 of 50", argv)
	}
}

func TestGHClient_SearchPRsPaged_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api search/issues", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().SearchPRsPaged(context.Background(), "", "", 1, 10)
		if err == nil || !strings.Contains(err.Error(), "search PRs") {
			t.Fatalf("err = %v, want a wrapped search error", err)
		}
	})
	t.Run("unparseable envelope", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api search/issues", Stdout: "nope"})
		_, err := NewGHClient().SearchPRsPaged(context.Background(), "", "", 1, 10)
		if err == nil || !strings.Contains(err.Error(), "parse PR search response") {
			t.Fatalf("err = %v, want an envelope parse failure", err)
		}
	})
	t.Run("unparseable items", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api search/issues", Stdout: `{"total_count":1,"items":{}}`})
		_, err := NewGHClient().SearchPRsPaged(context.Background(), "", "", 1, 10)
		if err == nil || !strings.Contains(err.Error(), "parse search results") {
			t.Fatalf("err = %v, want an items parse failure", err)
		}
	})
}

func TestGHClient_ListIssuesPaged_DropsPullRequests(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api search/issues",
		Stdout: `{"total_count":3,"items":[
			{"id":10,"node_id":"I_a","number":10,"title":"a real issue","body":"details",
			 "html_url":"https://github.com/o/r/issues/10","state":"open",
			 "created_at":"2026-05-01T00:00:00Z","updated_at":"2026-05-02T00:00:00Z",
			 "repository_url":"https://api.github.com/repos/o/r","user":{"login":"frank"},
			 "labels":[{"name":"bug"}],"assignees":[{"login":"grace"}]},
			{"id":11,"number":11,"title":"a PR","state":"open",
			 "repository_url":"https://api.github.com/repos/o/r","pull_request":{"url":"u"}}]}`,
	})

	page, err := NewGHClient().ListIssuesPaged(context.Background(), "label:bug", "", 1, 20)
	if err != nil {
		t.Fatalf("ListIssuesPaged: %v", err)
	}
	if !containsPair(calls(t)[0], "-f", "q=type:issue label:bug") {
		t.Errorf("argv = %q, want the injected type:issue qualifier", calls(t)[0])
	}
	if len(page.Issues) != 1 {
		t.Fatalf("issues = %#v, want the pull_request item dropped", page.Issues)
	}
	got := page.Issues[0]
	if got.ID != 10 || got.NodeID != "I_a" || got.Number != 10 || got.Title != "a real issue" {
		t.Errorf("issue = %#v", got)
	}
	if got.Body != "details" || got.RepoOwner != "o" || got.RepoName != "r" {
		t.Errorf("issue = %#v", got)
	}
	if strings.Join(got.Labels, ",") != "bug" || strings.Join(got.Assignees, ",") != "grace" {
		t.Errorf("labels/assignees = (%v, %v)", got.Labels, got.Assignees)
	}
	if page.TotalCount != 3 || page.Page != 1 || page.PerPage != 20 {
		t.Errorf("page meta = (%d, %d, %d)", page.TotalCount, page.Page, page.PerPage)
	}
}

func TestGHClient_ListIssues_UsesTheDefaultFirstPage(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api search/issues",
		Stdout: `{"total_count":0,"items":[]}`,
	})
	issues, err := NewGHClient().ListIssues(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("issues = %#v, want empty", issues)
	}
	argv := calls(t)[0]
	if !containsPair(argv, "-f", "per_page=50") || !containsPair(argv, "-f", "page=1") {
		t.Errorf("argv = %q, want the default page 1 of 50", argv)
	}
}

func TestGHClient_ListIssuesPaged_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api search/issues", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().ListIssuesPaged(context.Background(), "", "", 1, 10)
		if err == nil || !strings.Contains(err.Error(), "list issues") {
			t.Fatalf("err = %v, want a wrapped list-issues error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api search/issues", Stdout: "nope"})
		_, err := NewGHClient().ListIssuesPaged(context.Background(), "", "", 1, 10)
		if err == nil || !strings.Contains(err.Error(), "parse issue search response") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_GetIssueState(t *testing.T) {
	calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "closed\n"})
	state, err := NewGHClient().GetIssueState(context.Background(), "acme", "widget", 7)
	if err != nil {
		t.Fatalf("GetIssueState: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "repos/acme/widget/issues/7", "--jq", ".state",
	})
	if state != "closed" {
		t.Errorf("state = %q, want the trimmed closed", state)
	}
}

func TestGHClient_GetIssueState_Error(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1})
	state, err := NewGHClient().GetIssueState(context.Background(), "acme", "widget", 7)
	if state != "" {
		t.Errorf("state = %q, want empty", state)
	}
	if err == nil || !strings.Contains(err.Error(), "get issue state") {
		t.Fatalf("err = %v, want a wrapped error", err)
	}
}

func TestGHClient_HasRepositoryAccess(t *testing.T) {
	t.Run("readable repo", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api /repos/", Stdout: "acme/widget\n"})
		ok, err := NewGHClient().HasRepositoryAccess(context.Background(), "acme", "widget")
		if err != nil {
			t.Fatalf("HasRepositoryAccess: %v", err)
		}
		if !ok {
			t.Error("access = false, want true")
		}
		assertGHArgv(t, calls(t), 0, []string{"api", "/repos/acme/widget", "--jq", ".full_name"})
	})
	t.Run("blank output means no access", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api /repos/", Stdout: "\n"})
		ok, err := NewGHClient().HasRepositoryAccess(context.Background(), "acme", "widget")
		if err != nil {
			t.Fatalf("HasRepositoryAccess: %v", err)
		}
		if ok {
			t.Error("access = true, want false for empty output")
		}
	})
	t.Run("command failure propagates", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api /repos/", Stderr: "HTTP 404", Exit: 1})
		ok, err := NewGHClient().HasRepositoryAccess(context.Background(), "acme", "widget")
		if ok {
			t.Error("access = true, want false on error")
		}
		if err == nil {
			t.Fatal("expected the gh failure to propagate")
		}
	})
}

func TestGHClient_ListUserOrgs(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api user/orgs",
		Stdout: `[{"login":"acme","avatar_url":"https://avatars/acme"},{"login":"other"}]`,
	})
	orgs, err := NewGHClient().ListUserOrgs(context.Background())
	if err != nil {
		t.Fatalf("ListUserOrgs: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{"api", "user/orgs", "--paginate"})
	if len(orgs) != 2 {
		t.Fatalf("orgs = %#v, want 2", orgs)
	}
	if orgs[0] != (GitHubOrg{Login: "acme", AvatarURL: "https://avatars/acme"}) {
		t.Errorf("orgs[0] = %#v", orgs[0])
	}
	if orgs[1] != (GitHubOrg{Login: "other"}) {
		t.Errorf("orgs[1] = %#v", orgs[1])
	}
}

func TestGHClient_ListUserOrgs_Errors(t *testing.T) {
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api user/orgs", Stderr: "boom", Exit: 1})
		orgs, err := NewGHClient().ListUserOrgs(context.Background())
		if orgs != nil {
			t.Errorf("orgs = %#v, want nil", orgs)
		}
		if err == nil || !strings.Contains(err.Error(), "list user orgs") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api user/orgs", Stdout: "nope"})
		_, err := NewGHClient().ListUserOrgs(context.Background())
		if err == nil || !strings.Contains(err.Error(), "parse orgs") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

// containsPair reports whether argv holds `flag` immediately followed by value.
func containsPair(argv []string, flag, value string) bool {
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

func TestGHClient_SearchPRs_PropagatesTheUnderlyingFailure(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api search/issues", Stderr: "boom", Exit: 1})
	prs, err := NewGHClient().SearchPRs(context.Background(), "", "")
	if prs != nil {
		t.Errorf("prs = %#v, want nil", prs)
	}
	if err == nil || !strings.Contains(err.Error(), "search PRs") {
		t.Fatalf("err = %v, want the paged error surfaced unchanged", err)
	}
}

func TestGHClient_ListIssues_PropagatesTheUnderlyingFailure(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api search/issues", Stderr: "boom", Exit: 1})
	issues, err := NewGHClient().ListIssues(context.Background(), "", "")
	if issues != nil {
		t.Errorf("issues = %#v, want nil", issues)
	}
	if err == nil || !strings.Contains(err.Error(), "list issues") {
		t.Fatalf("err = %v, want the paged error surfaced unchanged", err)
	}
}

func TestGHClient_GetPRCommitDetail_Errors(t *testing.T) {
	const sha = "2222222222222222222222222222222222222222"
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().GetPRCommitDetail(context.Background(), "acme", "widget", sha)
		if err == nil || !strings.Contains(err.Error(), "get PR commit detail") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "nope"})
		_, err := NewGHClient().GetPRCommitDetail(context.Background(), "acme", "widget", sha)
		if err == nil || !strings.Contains(err.Error(), "parse PR commit detail") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_RequestReviewers_FailureNamesThePR(t *testing.T) {
	newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "HTTP 422", Exit: 1})
	err := NewGHClient().RequestReviewers(context.Background(), "acme", "widget", 42, []string{"octocat"})
	if err == nil || !strings.Contains(err.Error(), "request reviewers on PR #42") {
		t.Fatalf("err = %v, want the PR named in the wrap", err)
	}
}

// gh pr view exposes the head repository under several shapes. nameWithOwner
// backfills a missing owner/name, and httpsUrl is the fallback when cloneUrl
// is absent — without them a fork PR loses the remote it must be pushed to.
func TestConvertGHPR_HeadRepositoryFallbacks(t *testing.T) {
	t.Run("nameWithOwner backfills a missing owner and name", func(t *testing.T) {
		raw := &ghPR{Number: 1, State: "OPEN"}
		raw.HeadRepository.NameWithOwner = "contributor/widget-fork"
		raw.HeadRepository.HTTPSURL = "https://github.com/contributor/widget-fork.git"

		pr := convertGHPR(raw, "acme", "widget")
		if pr.HeadRepoOwner != "contributor" || pr.HeadRepoName != "widget-fork" {
			t.Errorf("head repo = (%q, %q), want it split out of nameWithOwner",
				pr.HeadRepoOwner, pr.HeadRepoName)
		}
		if pr.HeadRepoCloneURL != "https://github.com/contributor/widget-fork.git" {
			t.Errorf("clone url = %q, want the httpsUrl fallback", pr.HeadRepoCloneURL)
		}
	})
	t.Run("explicit fields win over nameWithOwner", func(t *testing.T) {
		raw := &ghPR{Number: 1, State: "OPEN"}
		raw.HeadRepositoryOwner.Login = "real-owner"
		raw.HeadRepository.Name = "real-name"
		raw.HeadRepository.NameWithOwner = "stale/stale-name"
		raw.HeadRepository.CloneURL = "https://github.com/real-owner/real-name.git"
		raw.HeadRepository.HTTPSURL = "https://ignored"

		pr := convertGHPR(raw, "acme", "widget")
		if pr.HeadRepoOwner != "real-owner" || pr.HeadRepoName != "real-name" {
			t.Errorf("head repo = (%q, %q)", pr.HeadRepoOwner, pr.HeadRepoName)
		}
		if pr.HeadRepoCloneURL != "https://github.com/real-owner/real-name.git" {
			t.Errorf("clone url = %q, want cloneUrl to win", pr.HeadRepoCloneURL)
		}
	})
	// gh pr view never returns baseRepository, so the --repo arguments are the
	// only trusted target identity and the base default branch stays unknown.
	t.Run("the base repo comes from the caller, not the payload", func(t *testing.T) {
		pr := convertGHPR(&ghPR{Number: 1, State: "OPEN", BaseRefName: "release/2.0"}, "acme", "widget")
		if pr.BaseRepoOwner != "acme" || pr.BaseRepoName != "widget" {
			t.Errorf("base repo = (%q, %q), want the caller's acme/widget", pr.BaseRepoOwner, pr.BaseRepoName)
		}
		if pr.BaseDefaultBranch != "" {
			t.Errorf("base default branch = %q, want empty — the base ref is not the default branch",
				pr.BaseDefaultBranch)
		}
	})
}

func TestConvertGHRequestedReviewers_SkipsAnEmptyTeam(t *testing.T) {
	got := convertGHRequestedReviewers([]ghRequestedReviewer{
		{TypeName: "Team"},
		{TypeName: "User", Login: "alice"},
	})
	if len(got) != 1 {
		t.Fatalf("reviewers = %#v, want the nameless team dropped", got)
	}
	if got[0] != (RequestedReviewer{Login: "alice", Type: reviewerTypeUser}) {
		t.Errorf("reviewers[0] = %#v", got[0])
	}
}
