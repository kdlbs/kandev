package github

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPATClient_ListRepoDirectory(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/contents/docs/specs": `[
			{"name":"spec.md","path":"docs/specs/spec.md","type":"file","sha":"aaa","size":128},
			{"name":"images","path":"docs/specs/images","type":"dir","sha":"bbb","size":0}
		]`,
	})

	entries, err := c.ListRepoDirectory(context.Background(), "acme", "widget", "docs/specs", "main")
	if err != nil {
		t.Fatalf("ListRepoDirectory: %v", err)
	}
	want := []RepoContentEntry{
		{Name: "spec.md", Path: "docs/specs/spec.md", Type: RepoContentTypeFile, SHA: "aaa", Size: 128},
		{Name: "images", Path: "docs/specs/images", Type: RepoContentTypeDir, SHA: "bbb"},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entries[%d] = %#v, want %#v", i, entries[i], want[i])
		}
	}
	if got := parseQueryValues(t, (*requests)[0].Query).Get("ref"); got != "main" {
		t.Errorf("ref = %q, want main", got)
	}
}

func TestPATClient_ListRepoDirectory_RootOmitsThePathSegment(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/contents": `[]`,
	})
	if _, err := c.ListRepoDirectory(context.Background(), "acme", "widget", "", ""); err != nil {
		t.Fatalf("ListRepoDirectory: %v", err)
	}
	if (*requests)[0].Path != "/repos/acme/widget/contents" {
		t.Errorf("path = %q, want the bare contents endpoint", (*requests)[0].Path)
	}
	if (*requests)[0].Query != "" {
		t.Errorf("query = %q, want no ref parameter", (*requests)[0].Query)
	}
}

func TestPATClient_ListRepoDirectory_MissingDirectoryIsTyped(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	entries, err := c.ListRepoDirectory(context.Background(), "acme", "widget", "nope", "main")
	if entries != nil {
		t.Errorf("entries = %#v, want nil", entries)
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want an unwrappable *GitHubAPIError with 404", err)
	}
	if !strings.Contains(err.Error(), "list repo directory") {
		t.Errorf("err = %v, want the wrap to name the operation", err)
	}
}

func TestPATClient_GetRepoFileContent(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	// GitHub wraps base64 content at 60 characters; the decoder must strip
	// those newlines before decoding. `\n` here is the two-character JSON
	// escape, so the decoded string really does carry an embedded newline.
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	wrapped := encoded[:8] + `\n` + encoded[8:]
	c, requests := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/contents/cmd/main.go": `{"type":"file","encoding":"base64","content":"` + wrapped + `"}`,
	})

	got, err := c.GetRepoFileContent(context.Background(), "acme", "widget", "cmd/main.go", "abc123")
	if err != nil {
		t.Fatalf("GetRepoFileContent: %v", err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}
	if got := parseQueryValues(t, (*requests)[0].Query).Get("ref"); got != "abc123" {
		t.Errorf("ref = %q, want abc123", got)
	}
}

func TestPATClient_GetRepoFileContent_RejectsUnsupportedEncoding(t *testing.T) {
	c, _ := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/contents/big.bin": `{"type":"file","encoding":"none","content":""}`,
	})
	got, err := c.GetRepoFileContent(context.Background(), "acme", "widget", "big.bin", "")
	if got != nil {
		t.Errorf("content = %q, want nil", got)
	}
	if err == nil || !strings.Contains(err.Error(), `unsupported encoding "none"`) {
		t.Fatalf("err = %v, want it to name the unsupported encoding", err)
	}
}

func TestPATClient_GetRepoFileContent_RejectsUndecodableBase64(t *testing.T) {
	c, _ := newRecordingPATServer(t, map[string]string{
		"/repos/acme/widget/contents/x": `{"type":"file","encoding":"base64","content":"!!!not base64!!!"}`,
	})
	if _, err := c.GetRepoFileContent(context.Background(), "acme", "widget", "x", ""); err == nil ||
		!strings.Contains(err.Error(), "decode base64") {
		t.Fatalf("err = %v, want a base64 decode failure", err)
	}
}

func TestPATClient_GetRepoFileContent_MissingFileIsTyped(t *testing.T) {
	c, _ := newRecordingPATServer(t, nil)
	_, err := c.GetRepoFileContent(context.Background(), "acme", "widget", "gone.txt", "")
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want an unwrappable *GitHubAPIError with 404", err)
	}
}

func TestPATClient_GetPRStatus_RollsUpReviewsAndChecks(t *testing.T) {
	routes := prStatusRoutes()
	routes["/repos/acme/widget/pulls/42/reviews"] = `[
		{"id":1,"state":"APPROVED","submitted_at":"2026-01-05T10:00:00Z","user":{"login":"alice"}},
		{"id":2,"state":"APPROVED","submitted_at":"2026-01-06T10:00:00Z","user":{"login":"bob"}}
	]`
	c, _ := newRecordingPATServer(t, routes)

	status, err := c.GetPRStatus(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("GetPRStatus: %v", err)
	}
	if status.PR == nil || status.PR.Number != 42 {
		t.Fatalf("PR = %#v", status.PR)
	}
	if status.ReviewState != computedReviewStateApproved {
		t.Errorf("review state = %q, want approved", status.ReviewState)
	}
	if status.ReviewCount != 2 {
		t.Errorf("review count = %d, want 2 distinct approving reviewers", status.ReviewCount)
	}
	if status.PendingReviewCount != 0 {
		t.Errorf("pending review count = %d, want 0", status.PendingReviewCount)
	}
	if status.ChecksState != "failure" {
		t.Errorf("checks state = %q, want failure", status.ChecksState)
	}
	if status.ChecksTotal != 2 || status.ChecksPassing != 1 {
		t.Errorf("checks = %d/%d, want 1/2", status.ChecksPassing, status.ChecksTotal)
	}
	if status.MergeableState != "blocked" {
		t.Errorf("mergeable state = %q, want blocked", status.MergeableState)
	}
	if !status.ChecksPopulated || !status.ReviewCountsPopulated {
		t.Error("the REST path counted checks and reviews; both Populated flags must be set")
	}
	// The REST path never fetches review threads, so it must not claim to
	// have counted them — SyncTaskPR relies on that to preserve the GraphQL value.
	if status.UnresolvedReviewThreadsPopulated {
		t.Error("UnresolvedReviewThreadsPopulated must stay false on the REST path")
	}
}

// ReviewState and ReviewCount deliberately answer different questions, and an
// approver who later leaves a plain comment is where they diverge:
// ReviewState takes each author's strictly-latest review (COMMENTED, so the
// PR is no longer "all approved"), while ReviewCount ignores non-binding
// states so the popover still renders "Approved (1)".
func TestPATClient_GetPRStatus_LaterCommentFromAnApprover(t *testing.T) {
	routes := prStatusRoutes()
	routes["/repos/acme/widget/pulls/42/reviews"] = `[
		{"id":1,"state":"APPROVED","submitted_at":"2026-01-05T10:00:00Z","user":{"login":"alice"}},
		{"id":2,"state":"COMMENTED","submitted_at":"2026-01-06T10:00:00Z","user":{"login":"alice"}}
	]`
	c, _ := newRecordingPATServer(t, routes)

	status, err := c.GetPRStatus(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("GetPRStatus: %v", err)
	}
	if status.ReviewState != computedReviewStatePending {
		t.Errorf("review state = %q, want pending", status.ReviewState)
	}
	if status.ReviewCount != 1 {
		t.Errorf("review count = %d, want 1 — a later comment must not drop the approval", status.ReviewCount)
	}
	if status.PendingReviewCount != 1 {
		t.Errorf("pending review count = %d, want 1", status.PendingReviewCount)
	}
}

// A requested-but-not-yet-submitted reviewer is the pending signal that wins
// over any derived-from-reviews count.
func TestPATClient_GetPRStatus_RequestedReviewersDrivePendingCount(t *testing.T) {
	routes := prStatusRoutes()
	routes["/repos/acme/widget/pulls/42"] = `{"number":42,"state":"open","mergeable_state":"BLOCKED",
		"requested_reviewers":[{"login":"carol"},{"login":"dave"}],
		"user":{"login":"bob"},"head":{"ref":"feature","sha":"headsha"},"base":{"ref":"main"}}`
	routes["/repos/acme/widget/pulls/42/reviews"] = `[]`
	c, _ := newRecordingPATServer(t, routes)

	status, err := c.GetPRStatus(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("GetPRStatus: %v", err)
	}
	if status.PendingReviewCount != 2 {
		t.Errorf("pending review count = %d, want 2 requested reviewers", status.PendingReviewCount)
	}
	if status.ReviewState != computedReviewStatePending {
		t.Errorf("review state = %q, want pending", status.ReviewState)
	}
	if status.ReviewCount != 0 {
		t.Errorf("review count = %d, want 0 — nobody has approved yet", status.ReviewCount)
	}
}

func TestPATClient_GetPRFeedback_AggregatesReviewsCommentsAndChecks(t *testing.T) {
	routes := prStatusRoutes()
	routes["/repos/acme/widget/pulls/42/comments"] = `[{"id":20,"body":"inline",
		"created_at":"2026-01-05T12:00:00Z","user":{"login":"alice","type":"User"}}]`
	routes["/repos/acme/widget/issues/42/comments"] = `[]`
	c, _ := newRecordingPATServer(t, routes)

	feedback, err := c.GetPRFeedback(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("GetPRFeedback: %v", err)
	}
	if feedback.PR == nil || feedback.PR.Number != 42 {
		t.Fatalf("PR = %#v", feedback.PR)
	}
	if len(feedback.Reviews) != 2 || len(feedback.Checks) != 2 {
		t.Errorf("reviews/checks = (%d, %d), want (2, 2)", len(feedback.Reviews), len(feedback.Checks))
	}
	if len(feedback.Comments) != 1 || feedback.Comments[0].ID != 20 {
		t.Errorf("comments = %#v", feedback.Comments)
	}
	// A failing check alone is enough to flag the PR as needing attention.
	if !feedback.HasIssues {
		t.Error("HasIssues = false, want true — one check concluded failure")
	}
}

func TestPATClient_GetPRFeedback_PropagatesAFailedLeg(t *testing.T) {
	routes := prStatusRoutes()
	// Drop the check-runs route so that leg 404s while the others succeed.
	delete(routes, "/repos/acme/widget/commits/headsha/check-runs")
	routes["/repos/acme/widget/pulls/42/comments"] = `[]`
	routes["/repos/acme/widget/issues/42/comments"] = `[]`
	c, _ := newRecordingPATServer(t, routes)

	feedback, err := c.GetPRFeedback(context.Background(), "acme", "widget", 42)
	if err == nil {
		t.Fatal("expected the failing check-runs leg to abort the aggregate")
	}
	if feedback != nil {
		t.Errorf("feedback = %#v, want nil", feedback)
	}
}

// prStatusRoutes is the four-endpoint fixture the PR status/feedback rollups
// read: the PR, its reviews, its check runs, and its commit status contexts.
func prStatusRoutes() map[string]string {
	return map[string]string{
		"/repos/acme/widget/pulls/42": `{"number":42,"state":"open","mergeable_state":"BLOCKED",
			"user":{"login":"bob"},"head":{"ref":"feature","sha":"headsha"},"base":{"ref":"main"}}`,
		"/repos/acme/widget/pulls/42/reviews": `[
			{"id":1,"state":"APPROVED","submitted_at":"2026-01-05T10:00:00Z","user":{"login":"alice"}},
			{"id":2,"state":"COMMENTED","submitted_at":"2026-01-06T10:00:00Z","user":{"login":"alice"}}
		]`,
		"/repos/acme/widget/commits/headsha/check-runs": `{"check_runs":[
			{"name":"unit","status":"completed","conclusion":"success","html_url":"https://ci/unit"},
			{"name":"lint","status":"completed","conclusion":"failure","html_url":"https://ci/lint"}
		]}`,
		"/repos/acme/widget/commits/headsha/status": `{"statuses":[]}`,
	}
}

func TestPATClient_ContextCancellationSurfaces(t *testing.T) {
	c, requests := newRecordingPATServer(t, map[string]string{"/user": `{"login":"octocat"}`})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.GetAuthenticatedUser(ctx)
	if err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want it to unwrap to context.Canceled", err)
	}
	if len(*requests) != 0 {
		t.Errorf("requests = %d, want 0 — a cancelled context must not reach the server", len(*requests))
	}
}

func TestParseNextLink(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"empty header", "", ""},
		{
			"next link is stripped down to path and query",
			`<` + githubAPIBase + `/repos/o/r/branches?per_page=100&page=2>; rel="next"`,
			"/repos/o/r/branches?per_page=100&page=2",
		},
		{
			"next is picked out of a multi-rel header",
			`<` + githubAPIBase + `/x?page=2>; rel="next", <` + githubAPIBase + `/x?page=9>; rel="last"`,
			"/x?page=2",
		},
		{
			"a last-only header has no next page",
			`<` + githubAPIBase + `/x?page=9>; rel="last"`,
			"",
		},
		{
			"an absolute URL off another host is returned verbatim",
			`<https://ghe.example.com/api/v3/x?page=2>; rel="next"`,
			"https://ghe.example.com/api/v3/x?page=2",
		},
		{"malformed brackets are skipped", `https://api.github.com/x?page=2; rel="next"`, ""},
		{"reversed brackets are skipped", `>` + githubAPIBase + `/x<; rel="next"`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseNextLink(tc.header); got != tc.want {
				t.Errorf("parseNextLink(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestFilterReposByQuery(t *testing.T) {
	repos := []GitHubRepo{
		{FullName: "acme/Widget"},
		{FullName: "acme/gadget"},
		{FullName: "other/widget-tools"},
	}
	t.Run("empty query returns the input unchanged", func(t *testing.T) {
		got := filterReposByQuery(repos, "")
		if len(got) != 3 {
			t.Fatalf("repos = %#v, want all three", got)
		}
	})
	t.Run("match is case-insensitive over full_name", func(t *testing.T) {
		got := filterReposByQuery(repos, "WIDGET")
		if len(got) != 2 {
			t.Fatalf("repos = %#v, want acme/Widget and other/widget-tools", got)
		}
		if got[0].FullName != "acme/Widget" || got[1].FullName != "other/widget-tools" {
			t.Errorf("repos = %#v", got)
		}
	})
	t.Run("no match yields an empty, non-nil slice", func(t *testing.T) {
		got := filterReposByQuery(repos, "nothing")
		if got == nil || len(got) != 0 {
			t.Fatalf("repos = %#v, want an empty slice", got)
		}
	})
}

func TestConvertRepoListItems(t *testing.T) {
	pushed := time.Date(2026, 5, 4, 3, 2, 1, 0, time.UTC)
	items := []repoListItem{
		{
			FullName: "acme/widget", Name: "widget", Private: true,
			DefaultBranch: "trunk", Description: "a widget", PushedAt: pushed,
		},
		{FullName: "acme/quiet", Name: "quiet"},
	}
	items[0].Owner.Login = "acme"
	items[1].Owner.Login = "acme"

	got := convertRepoListItems(items)
	if len(got) != 2 {
		t.Fatalf("repos = %d, want 2", len(got))
	}
	if got[0].FullName != "acme/widget" || got[0].Owner != "acme" || got[0].Name != "widget" {
		t.Errorf("repos[0] identity = %#v", got[0])
	}
	if !got[0].Private || got[0].DefaultBranch != "trunk" || got[0].Description != "a widget" {
		t.Errorf("repos[0] attributes = %#v", got[0])
	}
	if got[0].PushedAt == nil || !got[0].PushedAt.Equal(pushed) {
		t.Errorf("repos[0] pushed_at = %v, want %v", got[0].PushedAt, pushed)
	}
	// A zero pushed_at must stay nil so `omitempty` drops it instead of
	// serializing "0001-01-01T00:00:00Z".
	if got[1].PushedAt != nil {
		t.Errorf("repos[1] pushed_at = %v, want nil", got[1].PushedAt)
	}
}
