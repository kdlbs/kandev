package github

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGHClient_IsAuthenticated(t *testing.T) {
	t.Run("gh auth status succeeds", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "auth status", Stdout: "Logged in to github.com\n"})
		ok, err := NewGHClient().IsAuthenticated(context.Background())
		if err != nil {
			t.Fatalf("IsAuthenticated: %v", err)
		}
		if !ok {
			t.Error("authenticated = false, want true")
		}
		assertGHArgv(t, calls(t), 0, []string{"auth", "status", "--hostname", "github.com"})
	})
	// Any non-zero exit is "not authenticated" — the message text is
	// locale-dependent, so it is deliberately not parsed.
	t.Run("a non-zero exit reports false without an error", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "auth status", Stderr: "You are not logged in", Exit: 1})
		ok, err := NewGHClient().IsAuthenticated(context.Background())
		if err != nil {
			t.Fatalf("a plain auth failure must not surface an error, got %v", err)
		}
		if ok {
			t.Error("authenticated = true, want false")
		}
	})
	// Cancellation must stay distinguishable from "not authenticated" so
	// callers don't treat a shutdown as a lost login.
	t.Run("cancellation propagates", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "auth status", Stdout: "ok"})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ok, err := NewGHClient().IsAuthenticated(ctx)
		if ok {
			t.Error("authenticated = true, want false")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want it to unwrap to context.Canceled", err)
		}
	})
}

func TestGHClient_RunAuthDiagnostics(t *testing.T) {
	t.Run("success captures stdout when stderr is empty", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "auth status", Stdout: "Logged in as octocat"})
		got := NewGHClient().RunAuthDiagnostics(context.Background())
		assertGHArgv(t, calls(t), 0, []string{"auth", "status", "--hostname", "github.com"})
		if got.Command != "gh auth status --hostname github.com" {
			t.Errorf("command = %q", got.Command)
		}
		if got.Output != "Logged in as octocat" {
			t.Errorf("output = %q, want the stdout text", got.Output)
		}
		if got.ExitCode != 0 {
			t.Errorf("exit code = %d, want 0", got.ExitCode)
		}
	})
	// gh writes its auth report to stderr, so stderr wins when both are set.
	t.Run("stderr is preferred over stdout", func(t *testing.T) {
		newFakeGH(t, ghResponse{
			Prefix: "auth status", Stdout: "ignored", Stderr: "You are not logged into any hosts", Exit: 1,
		})
		got := NewGHClient().RunAuthDiagnostics(context.Background())
		if got.Output != "You are not logged into any hosts" {
			t.Errorf("output = %q, want the stderr text", got.Output)
		}
		if got.ExitCode != 1 {
			t.Errorf("exit code = %d, want 1", got.ExitCode)
		}
	})
	// A silent non-zero exit would otherwise render as a blank diagnostics
	// panel, so the underlying error is surfaced as the output.
	t.Run("a silent failure surfaces the underlying error", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "auth status", Exit: 3})
		got := NewGHClient().RunAuthDiagnostics(context.Background())
		if got.ExitCode != 3 {
			t.Errorf("exit code = %d, want 3", got.ExitCode)
		}
		if !strings.Contains(got.Output, "gh auth status did not run") {
			t.Errorf("output = %q, want the did-not-run explanation", got.Output)
		}
	})
	t.Run("a cancelled context reports the context error too", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "auth status", Stdout: "ok"})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got := NewGHClient().RunAuthDiagnostics(ctx)
		if got.ExitCode != -1 {
			t.Errorf("exit code = %d, want -1 for a non-exec failure", got.ExitCode)
		}
		if !strings.Contains(got.Output, "context:") {
			t.Errorf("output = %q, want the context cause included", got.Output)
		}
	})
}

func TestGHClient_FetchRateLimit(t *testing.T) {
	t.Run("no tracker is a no-op that never execs gh", func(t *testing.T) {
		calls := newFakeGH(t, ghResponse{Prefix: "api rate_limit", Stdout: `{}`})
		if err := NewGHClient().FetchRateLimit(context.Background()); err != nil {
			t.Fatalf("FetchRateLimit: %v", err)
		}
		if len(calls(t)) != 0 {
			t.Errorf("gh calls = %d, want 0 without a tracker", len(calls(t)))
		}
	})
	t.Run("seeds every reported bucket", func(t *testing.T) {
		reset := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
		calls := newFakeGH(t, ghResponse{
			Prefix: "api rate_limit",
			Stdout: `{"resources":{
				"core":{"limit":5000,"remaining":4990,"reset":` + strconv.FormatInt(reset.Unix(), 10) + `},
				"graphql":{"limit":5000,"remaining":4000,"reset":` + strconv.FormatInt(reset.Unix(), 10) + `},
				"search":{"limit":30,"remaining":29,"reset":` + strconv.FormatInt(reset.Unix(), 10) + `}}}`,
		})
		tracker := NewRateTracker(nil, nil)
		if err := NewGHClient().WithRateTracker(tracker).FetchRateLimit(context.Background()); err != nil {
			t.Fatalf("FetchRateLimit: %v", err)
		}
		assertGHArgv(t, calls(t), 0, []string{"api", "rate_limit"})
		for _, want := range []struct {
			resource  Resource
			limit     int
			remaining int
		}{
			{ResourceCore, 5000, 4990},
			{ResourceGraphQL, 5000, 4000},
			{ResourceSearch, 30, 29},
		} {
			snap, ok := tracker.Snapshot(want.resource)
			if !ok {
				t.Errorf("%s: no snapshot recorded", want.resource)
				continue
			}
			if snap.Limit != want.limit || snap.Remaining != want.remaining {
				t.Errorf("%s = %d/%d, want %d/%d",
					want.resource, snap.Remaining, snap.Limit, want.remaining, want.limit)
			}
			if !snap.ResetAt.Equal(reset) {
				t.Errorf("%s reset = %v, want %v", want.resource, snap.ResetAt, reset)
			}
		}
	})
	// An all-zero bucket means gh reported nothing for that resource; recording
	// it would falsely present the bucket as exhausted.
	t.Run("all-zero buckets are skipped", func(t *testing.T) {
		newFakeGH(t, ghResponse{
			Prefix: "api rate_limit",
			Stdout: `{"resources":{"core":{"limit":5000,"remaining":10,"reset":1893456000},
				"graphql":{"limit":0,"remaining":0,"reset":0},
				"search":{"limit":0,"remaining":0,"reset":0}}}`,
		})
		tracker := NewRateTracker(nil, nil)
		if err := NewGHClient().WithRateTracker(tracker).FetchRateLimit(context.Background()); err != nil {
			t.Fatalf("FetchRateLimit: %v", err)
		}
		if _, ok := tracker.Snapshot(ResourceCore); !ok {
			t.Error("core snapshot missing")
		}
		if _, ok := tracker.Snapshot(ResourceGraphQL); ok {
			t.Error("graphql snapshot recorded, want the all-zero bucket skipped")
		}
		if _, ok := tracker.Snapshot(ResourceSearch); ok {
			t.Error("search snapshot recorded, want the all-zero bucket skipped")
		}
	})
	t.Run("command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api rate_limit", Stderr: "boom", Exit: 1})
		err := NewGHClient().WithRateTracker(NewRateTracker(nil, nil)).FetchRateLimit(context.Background())
		if err == nil || !strings.Contains(err.Error(), "fetch rate limit") {
			t.Fatalf("err = %v, want a wrapped fetch error", err)
		}
	})
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api rate_limit", Stdout: "nope"})
		err := NewGHClient().WithRateTracker(NewRateTracker(nil, nil)).FetchRateLimit(context.Background())
		if err == nil || !strings.Contains(err.Error(), "parse rate_limit response") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}

func TestGHClient_ListCheckRuns_Errors(t *testing.T) {
	t.Run("check-runs command failure", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api --paginate", Stderr: "boom", Exit: 1})
		_, err := NewGHClient().ListCheckRuns(context.Background(), "acme", "widget", "sha")
		if err == nil || !strings.Contains(err.Error(), "list check runs") {
			t.Fatalf("err = %v, want a wrapped check-runs error", err)
		}
	})
	t.Run("undecodable check-run stream", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api --paginate", Stdout: `{"name":"unit"`})
		_, err := NewGHClient().ListCheckRuns(context.Background(), "acme", "widget", "sha")
		if err == nil || !strings.Contains(err.Error(), "parse check runs") {
			t.Fatalf("err = %v, want a check-run decode failure", err)
		}
	})
	t.Run("status command failure", func(t *testing.T) {
		newFakeGH(t,
			ghResponse{Prefix: "api --paginate", Stdout: ""},
			ghResponse{Prefix: "api repos/", Stderr: "boom", Exit: 1},
		)
		_, err := NewGHClient().ListCheckRuns(context.Background(), "acme", "widget", "sha")
		if err == nil || !strings.Contains(err.Error(), "list status contexts") {
			t.Fatalf("err = %v, want a wrapped status error", err)
		}
	})
	t.Run("unparseable status contexts", func(t *testing.T) {
		newFakeGH(t,
			ghResponse{Prefix: "api --paginate", Stdout: ""},
			ghResponse{Prefix: "api repos/", Stdout: "nope"},
		)
		_, err := NewGHClient().ListCheckRuns(context.Background(), "acme", "widget", "sha")
		if err == nil || !strings.Contains(err.Error(), "parse status contexts") {
			t.Fatalf("err = %v, want a status parse failure", err)
		}
	})
}

// A commit status context and a check run sharing a name collapse into one
// entry, with the check run winning.
func TestGHClient_ListCheckRuns_MergesStatusContexts(t *testing.T) {
	newFakeGH(t,
		ghResponse{
			Prefix: "api --paginate",
			Stdout: `{"name":"unit","status":"completed","conclusion":"success","html_url":"https://ci/unit"}`,
		},
		ghResponse{
			Prefix: "api repos/",
			Stdout: `[{"context":"unit","state":"failure","target_url":"https://legacy/unit"},
				{"context":"legacy-only","state":"success","target_url":"https://legacy/other",
				 "description":"all good"}]`,
		},
	)

	checks, err := NewGHClient().ListCheckRuns(context.Background(), "acme", "widget", "sha")
	if err != nil {
		t.Fatalf("ListCheckRuns: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks = %#v, want the duplicate name merged into 2 entries", checks)
	}
	byName := map[string]CheckRun{}
	for _, c := range checks {
		byName[c.Name] = c
	}
	unit, ok := byName["unit"]
	if !ok {
		t.Fatal("unit check missing")
	}
	if unit.Source != checkSourceCheckRun || unit.Conclusion != "success" {
		t.Errorf("unit = %#v, want the check-run entry to win over the status context", unit)
	}
	legacy, ok := byName["legacy-only"]
	if !ok {
		t.Fatal("legacy-only check missing")
	}
	if legacy.Source != checkSourceStatusContext || legacy.Conclusion != checkStatusSuccess {
		t.Errorf("legacy-only = %#v", legacy)
	}
	if legacy.Output != "all good" {
		t.Errorf("legacy-only output = %q, want the status description", legacy.Output)
	}
}

func TestGHClient_GetPRStatus(t *testing.T) {
	newFakeGH(t, ghPRStatusResponses()...)

	status, err := NewGHClient().GetPRStatus(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("GetPRStatus: %v", err)
	}
	if status.PR == nil || status.PR.Number != 42 {
		t.Fatalf("PR = %#v", status.PR)
	}
	if status.ReviewState != computedReviewStateApproved || status.ReviewCount != 1 {
		t.Errorf("review = (%q, %d), want (approved, 1)", status.ReviewState, status.ReviewCount)
	}
	if status.ChecksState != "failure" {
		t.Errorf("checks state = %q, want failure", status.ChecksState)
	}
	if status.ChecksTotal != 2 || status.ChecksPassing != 1 {
		t.Errorf("checks = %d/%d, want 1/2", status.ChecksPassing, status.ChecksTotal)
	}
	if !status.ChecksPopulated || !status.ReviewCountsPopulated {
		t.Error("the gh REST path counted checks and reviews; both flags must be set")
	}
	if status.UnresolvedReviewThreadsPopulated {
		t.Error("UnresolvedReviewThreadsPopulated must stay false — no threads were fetched")
	}
}

func TestGHClient_GetPRFeedback(t *testing.T) {
	responses := append(ghPRStatusResponses(),
		ghResponse{
			Prefix: "api repos/acme/widget/pulls/42/comments",
			Stdout: `[{"id":20,"body":"inline","created_at":"2026-01-05T12:00:00Z",
				"user":{"login":"alice","type":"User"}}]`,
		},
		ghResponse{Prefix: "api repos/acme/widget/issues/42/comments", Stdout: `[]`},
	)
	newFakeGH(t, responses...)

	feedback, err := NewGHClient().GetPRFeedback(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("GetPRFeedback: %v", err)
	}
	if feedback.PR == nil || feedback.PR.Number != 42 {
		t.Fatalf("PR = %#v", feedback.PR)
	}
	if len(feedback.Reviews) != 1 || len(feedback.Checks) != 2 {
		t.Errorf("reviews/checks = (%d, %d), want (1, 2)", len(feedback.Reviews), len(feedback.Checks))
	}
	if len(feedback.Comments) != 1 || feedback.Comments[0].ID != 20 {
		t.Errorf("comments = %#v", feedback.Comments)
	}
	if !feedback.HasIssues {
		t.Error("HasIssues = false, want true — one check concluded failure")
	}
}

// ghPRStatusResponses is the dispatch set the PR status rollup needs: the PR
// itself, its reviews, its paginated check runs, and its commit statuses.
// The comment endpoints are matched before the bare `api repos/` arm, so
// callers appending them must place them ahead of nothing — the ordering here
// already puts the specific pulls/issues comment paths first when appended.
func ghPRStatusResponses() []ghResponse {
	return []ghResponse{
		{
			Prefix: "pr view",
			Stdout: `{"number":42,"state":"OPEN","mergeStateStatus":"BLOCKED",
				"headRefName":"feature","headRefOid":"headsha","baseRefName":"main",
				"author":{"login":"bob"}}`,
		},
		{
			Prefix: "api --paginate",
			Stdout: `{"name":"unit","status":"completed","conclusion":"success","html_url":"https://ci/unit"}
{"name":"lint","status":"completed","conclusion":"failure","html_url":"https://ci/lint"}`,
		},
		{
			Prefix: "api repos/acme/widget/pulls/42/reviews",
			Stdout: `[{"id":1,"state":"APPROVED","submitted_at":"2026-01-05T10:00:00Z",
				"user":{"login":"alice"}}]`,
		},
		{Prefix: "api repos/acme/widget/commits", Stdout: `[]`},
	}
}

func TestGHClient_ListRepoDirectory(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: `[{"name":"spec.md","path":"docs/spec.md","type":"file","sha":"aaa","size":128},
			{"name":"images","path":"docs/images","type":"dir","sha":"bbb"}]`,
	})

	entries, err := NewGHClient().ListRepoDirectory(context.Background(), "acme", "widget", "docs", "main")
	if err != nil {
		t.Fatalf("ListRepoDirectory: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "repos/acme/widget/contents/docs", "-X", "GET", "-f", "ref=main",
	})
	want := []RepoContentEntry{
		{Name: "spec.md", Path: "docs/spec.md", Type: RepoContentTypeFile, SHA: "aaa", Size: 128},
		{Name: "images", Path: "docs/images", Type: RepoContentTypeDir, SHA: "bbb"},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entries[%d] = %#v, want %#v", i, entries[i], want[i])
		}
	}
}

func TestGHClient_ListRepoDirectory_RootOmitsPathAndRef(t *testing.T) {
	calls := newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: `[]`})
	if _, err := NewGHClient().ListRepoDirectory(context.Background(), "acme", "widget", "", ""); err != nil {
		t.Fatalf("ListRepoDirectory: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{"api", "repos/acme/widget/contents", "-X", "GET"})
}

func TestGHClient_ListRepoDirectory_Errors(t *testing.T) {
	t.Run("404 becomes a typed error", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "gh: HTTP 404: Not Found", Exit: 1})
		entries, err := NewGHClient().ListRepoDirectory(context.Background(), "acme", "widget", "nope", "")
		if entries != nil {
			t.Errorf("entries = %#v, want nil", entries)
		}
		var apiErr *GitHubAPIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
			t.Fatalf("err = %v, want a *GitHubAPIError with 404", err)
		}
		if apiErr.Endpoint != "repos/acme/widget/contents/nope" {
			t.Errorf("endpoint = %q", apiErr.Endpoint)
		}
	})
	// 422 is not one of the statuses config sync's fetch classification needs
	// promoted (404/401/403/429/5xx), so it stays the residue case: any
	// status this client does not recognize falls through to a bare wrap
	// rather than being guessed at.
	t.Run("other failures stay untyped", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "HTTP 422", Exit: 1})
		_, err := NewGHClient().ListRepoDirectory(context.Background(), "acme", "widget", "docs", "")
		if err == nil || !strings.Contains(err.Error(), "list repo directory") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			t.Fatalf("err = %v, want it to stay untyped", err)
		}
	})
	for _, tt := range []struct {
		name   string
		stderr string
		want   int
	}{
		{"401 becomes a typed error", "gh: HTTP 401: Bad credentials", http.StatusUnauthorized},
		{"403 becomes a typed error", "gh: HTTP 403: Forbidden", http.StatusForbidden},
		{"429 becomes a typed error", "gh: HTTP 429: Too Many Requests", http.StatusTooManyRequests},
		{"5xx becomes a typed error", "gh: HTTP 503: Service Unavailable", http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: tt.stderr, Exit: 1})
			entries, err := NewGHClient().ListRepoDirectory(context.Background(), "acme", "widget", "docs", "")
			if entries != nil {
				t.Errorf("entries = %#v, want nil", entries)
			}
			var apiErr *GitHubAPIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.want {
				t.Fatalf("err = %v, want a *GitHubAPIError with %d", err, tt.want)
			}
		})
	}
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "nope"})
		_, err := NewGHClient().ListRepoDirectory(context.Background(), "acme", "widget", "docs", "")
		if err == nil || !strings.Contains(err.Error(), "list repo directory: decode") {
			t.Fatalf("err = %v, want a decode failure", err)
		}
	})
}

func TestGHClient_GetRepoFileContent(t *testing.T) {
	content := "package main\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	// GitHub wraps base64 at 60 chars; `\n` here is the JSON escape, so the
	// decoded field really carries an embedded newline that must be stripped.
	wrapped := encoded[:6] + `\n` + encoded[6:]
	calls := newFakeGH(t, ghResponse{
		Prefix: "api repos/",
		Stdout: `{"type":"file","encoding":"base64","content":"` + wrapped + `"}`,
	})

	got, err := NewGHClient().GetRepoFileContent(context.Background(), "acme", "widget", "cmd/main.go", "abc")
	if err != nil {
		t.Fatalf("GetRepoFileContent: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"api", "repos/acme/widget/contents/cmd/main.go", "-X", "GET", "-f", "ref=abc",
	})
	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestGHClient_GetRepoFileContent_Errors(t *testing.T) {
	t.Run("404 becomes a typed error", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "gh: HTTP 404: Not Found", Exit: 1})
		_, err := NewGHClient().GetRepoFileContent(context.Background(), "acme", "widget", "gone.txt", "")
		var apiErr *GitHubAPIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
			t.Fatalf("err = %v, want a *GitHubAPIError with 404", err)
		}
		if apiErr.Endpoint != "repos/acme/widget/contents/gone.txt" {
			t.Errorf("endpoint = %q", apiErr.Endpoint)
		}
	})
	// 422 stays the residue case for the same reason as
	// TestGHClient_ListRepoDirectory_Errors: it is not one of the statuses
	// config sync's fetch classification promotes.
	t.Run("other failures stay untyped", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: "HTTP 422", Exit: 1})
		_, err := NewGHClient().GetRepoFileContent(context.Background(), "acme", "widget", "x", "")
		if err == nil || !strings.Contains(err.Error(), "get repo file content") {
			t.Fatalf("err = %v, want a wrapped error", err)
		}
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			t.Fatalf("err = %v, want it to stay untyped", err)
		}
	})
	for _, tt := range []struct {
		name   string
		stderr string
		want   int
	}{
		{"401 becomes a typed error", "gh: HTTP 401: Bad credentials", http.StatusUnauthorized},
		{"403 becomes a typed error", "gh: HTTP 403: Forbidden", http.StatusForbidden},
		{"429 becomes a typed error", "gh: HTTP 429: Too Many Requests", http.StatusTooManyRequests},
		{"5xx becomes a typed error", "gh: HTTP 503: Service Unavailable", http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			newFakeGH(t, ghResponse{Prefix: "api repos/", Stderr: tt.stderr, Exit: 1})
			_, err := NewGHClient().GetRepoFileContent(context.Background(), "acme", "widget", "x", "")
			var apiErr *GitHubAPIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.want {
				t.Fatalf("err = %v, want a *GitHubAPIError with %d", err, tt.want)
			}
		})
	}
	t.Run("unparseable output", func(t *testing.T) {
		newFakeGH(t, ghResponse{Prefix: "api repos/", Stdout: "nope"})
		_, err := NewGHClient().GetRepoFileContent(context.Background(), "acme", "widget", "x", "")
		if err == nil || !strings.Contains(err.Error(), "get repo file content: decode") {
			t.Fatalf("err = %v, want a decode failure", err)
		}
	})
	t.Run("unsupported encoding", func(t *testing.T) {
		newFakeGH(t, ghResponse{
			Prefix: "api repos/", Stdout: `{"type":"file","encoding":"none","content":""}`,
		})
		_, err := NewGHClient().GetRepoFileContent(context.Background(), "acme", "widget", "x", "")
		if err == nil || !strings.Contains(err.Error(), `unsupported encoding "none"`) {
			t.Fatalf("err = %v, want the encoding named", err)
		}
	})
	t.Run("undecodable base64", func(t *testing.T) {
		newFakeGH(t, ghResponse{
			Prefix: "api repos/", Stdout: `{"type":"file","encoding":"base64","content":"!!!"}`,
		})
		_, err := NewGHClient().GetRepoFileContent(context.Background(), "acme", "widget", "x", "")
		if err == nil || !strings.Contains(err.Error(), "decode base64") {
			t.Fatalf("err = %v, want a base64 failure", err)
		}
	})
}

// The exec budget starts only after the throttle slot is acquired, and is
// capped at ghCLITimeout — a caller deadline further out must not extend it.
func TestResolveGHExecTimeout(t *testing.T) {
	t.Run("no deadline takes the full budget", func(t *testing.T) {
		if got := resolveGHExecTimeout(context.Background()); got != ghCLITimeout {
			t.Errorf("timeout = %v, want %v", got, ghCLITimeout)
		}
	})
	t.Run("a nearer deadline wins", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		got := resolveGHExecTimeout(ctx)
		if got > 2*time.Second || got <= 0 {
			t.Errorf("timeout = %v, want just under 2s", got)
		}
	})
	t.Run("a further deadline is capped", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		if got := resolveGHExecTimeout(ctx); got != ghCLITimeout {
			t.Errorf("timeout = %v, want it capped at %v", got, ghCLITimeout)
		}
	})
}

func TestFirstArg(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"nil", nil, "<no-args>"},
		{"empty", []string{}, "<no-args>"},
		{"single", []string{"api"}, "api"},
		{"multiple", []string{"pr", "view", "1"}, "pr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstArg(tc.args); got != tc.want {
				t.Errorf("firstArg(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestConvertGHIssue(t *testing.T) {
	raw := &ghIssue{
		Number:    11,
		Title:     "Crash on start",
		URL:       "https://github.com/acme/widget/issues/11",
		State:     "CLOSED",
		Body:      "stack trace",
		CreatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
		ClosedAt:  "2026-02-03T00:00:00Z",
	}
	raw.Author.Login = "erin"
	raw.Labels = []struct {
		Name string `json:"name"`
	}{{Name: "bug"}, {Name: "regression"}}
	raw.Assignees = []struct {
		Login string `json:"login"`
	}{{Login: "frank"}}

	issue := convertGHIssue(raw, "acme", "widget")
	if issue.Number != 11 || issue.Title != "Crash on start" || issue.Body != "stack trace" {
		t.Errorf("scalars = %#v", issue)
	}
	if issue.URL != raw.URL || issue.HTMLURL != raw.URL {
		t.Errorf("URL/HTMLURL = (%q, %q), want both %q", issue.URL, issue.HTMLURL, raw.URL)
	}
	if issue.State != "closed" {
		t.Errorf("state = %q, want lowercased", issue.State)
	}
	if issue.AuthorLogin != "erin" || issue.RepoOwner != "acme" || issue.RepoName != "widget" {
		t.Errorf("author/repo = (%q, %q, %q)", issue.AuthorLogin, issue.RepoOwner, issue.RepoName)
	}
	if strings.Join(issue.Labels, ",") != "bug,regression" {
		t.Errorf("labels = %v", issue.Labels)
	}
	if strings.Join(issue.Assignees, ",") != "frank" {
		t.Errorf("assignees = %v", issue.Assignees)
	}
	if !issue.CreatedAt.Equal(raw.CreatedAt) || !issue.UpdatedAt.Equal(raw.UpdatedAt) {
		t.Errorf("timestamps = (%v, %v)", issue.CreatedAt, issue.UpdatedAt)
	}
	wantClosed := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	if issue.ClosedAt == nil || !issue.ClosedAt.Equal(wantClosed) {
		t.Errorf("closed_at = %v, want %v", issue.ClosedAt, wantClosed)
	}
}

func TestGHClient_ParsePRList(t *testing.T) {
	t.Run("stamps the repo identity onto every row", func(t *testing.T) {
		prs, err := (&GHClient{}).parsePRList(
			`[{"number":1,"state":"OPEN","author":{"login":"a"}},
			  {"number":2,"state":"CLOSED","mergedAt":"2026-01-01T00:00:00Z","author":{"login":"b"}}]`,
			"acme", "widget",
		)
		if err != nil {
			t.Fatalf("parsePRList: %v", err)
		}
		if len(prs) != 2 {
			t.Fatalf("prs = %#v, want 2", prs)
		}
		for i, pr := range prs {
			if pr.RepoOwner != "acme" || pr.RepoName != "widget" {
				t.Errorf("prs[%d] repo = (%q, %q)", i, pr.RepoOwner, pr.RepoName)
			}
		}
		if prs[0].State != "open" {
			t.Errorf("prs[0] state = %q, want open", prs[0].State)
		}
		if prs[1].State != prStateMerged {
			t.Errorf("prs[1] state = %q, want merged", prs[1].State)
		}
	})
	t.Run("unparseable input", func(t *testing.T) {
		if _, err := (&GHClient{}).parsePRList("nope", "o", "r"); err == nil ||
			!strings.Contains(err.Error(), "parse PR list") {
			t.Fatalf("err = %v, want a parse failure", err)
		}
	})
}
