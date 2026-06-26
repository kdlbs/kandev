package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsNotFoundErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"HTTP 404", errors.New("gh api: HTTP 404"), true},
		{"HTTP 404 with suffix", errors.New("gh: HTTP 404: Not Found (404)"), true},
		{"404 Not Found", errors.New("404 Not Found"), true},
		{"status: 404", errors.New("request failed (status: 404)"), true},
		{"unrelated 500", errors.New("HTTP 500: server error"), false},
		{"unrelated text", errors.New("connection refused"), false},
		{"403 not 404", errors.New("HTTP 403: Forbidden"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNotFoundErr(tc.err); got != tc.want {
				t.Fatalf("isNotFoundErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsForbiddenErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"HTTP 403", errors.New("gh api: HTTP 403"), true},
		{"403 Forbidden", errors.New("403 Forbidden"), true},
		{"status: 403", errors.New("request failed (status: 403)"), true},
		{"404 not 403", errors.New("HTTP 404"), false},
		{"500 not 403", errors.New("HTTP 500"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isForbiddenErr(tc.err); got != tc.want {
				t.Fatalf("isForbiddenErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestGhMergeStatusCode(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantOK   bool
		wantCode int
	}{
		{"nil", nil, false, 0},
		{"unrelated", errors.New("connection refused"), false, 0},
		{"HTTP 404", errors.New("HTTP 404: Not Found"), true, 404},
		{"404 Not Found phrase", errors.New("404 Not Found"), true, 404},
		{"status 404", errors.New("request failed (status: 404)"), true, 404},
		{"HTTP 403", errors.New("HTTP 403: Forbidden"), true, 403},
		{"HTTP 405", errors.New("gh: HTTP 405: Method Not Allowed"), true, 405},
		{"status 405", errors.New("status: 405"), true, 405},
		{"405 phrase", errors.New("405 Method Not Allowed"), true, 405},
		{"HTTP 409", errors.New("HTTP 409: Conflict"), true, 409},
		{"status 409", errors.New("status: 409"), true, 409},
		{"409 phrase", errors.New("409 Conflict"), true, 409},
		{"500 not mapped", errors.New("HTTP 500: server error"), false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := ghMergeStatusCode(tc.err)
			if ok != tc.wantOK || code != tc.wantCode {
				t.Fatalf("ghMergeStatusCode(%v) = (%d, %v), want (%d, %v)",
					tc.err, code, ok, tc.wantCode, tc.wantOK)
			}
		})
	}
}

func TestParseTimePtr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
	}{
		{"empty string", "", true},
		{"valid RFC3339", "2025-01-15T10:30:00Z", false},
		{"invalid format", "not-a-date", true},
		{"date only", "2025-01-15", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimePtr(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil, got nil")
			}
		})
	}
}

func TestParseTimePtrValue(t *testing.T) {
	got := parseTimePtr("2025-06-15T14:30:00Z")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	expected := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)
	if !got.Equal(expected) {
		t.Errorf("got %v, want %v", *got, expected)
	}
}

func TestGHClient_ListCheckRuns_PaginatesCheckRuns(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh-args.log")
	ghPath := filepath.Join(binDir, "gh")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$GH_ARGS_LOG"
case "$*" in
  *check-runs*)
    case "$*" in
      *--slurp*) printf '%s\n' 'unsupported slurp' >&2; exit 1 ;;
      *--paginate*) printf '%s\n' '{"name":"unit","status":"completed","conclusion":"success","html_url":"https://ci/unit"}' '{"name":"lint","status":"completed","conclusion":"failure","html_url":"https://ci/lint"}' ;;
      *) printf '%s\n' '[{"name":"unit","status":"completed","conclusion":"success","html_url":"https://ci/unit"}]' ;;
    esac
    ;;
  *status*) printf '%s\n' '[]' ;;
  *) printf '%s\n' '[]' ;;
esac
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_ARGS_LOG", logPath)

	checks, err := NewGHClient().ListCheckRuns(context.Background(), "acme", "widget", "sha")
	if err != nil {
		t.Fatalf("ListCheckRuns: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks = %d, want 2: %+v", len(checks), checks)
	}
	if checks[1].Name != "lint" || checks[1].Conclusion != "failure" {
		t.Fatalf("expected failed lint check from paginated output, got %+v", checks[1])
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gh args log: %v", err)
	}
	if !strings.Contains(string(logged), "--paginate") {
		t.Fatalf("ListCheckRuns should call gh api with --paginate, got:\n%s", logged)
	}
	if strings.Contains(string(logged), "--slurp") {
		t.Fatalf("ListCheckRuns should not combine gh api --slurp with --jq, got:\n%s", logged)
	}
}

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
	}{
		{
			"standard API URL",
			"https://api.github.com/repos/myorg/myrepo",
			"myorg", "myrepo",
		},
		{
			"short path",
			"owner/repo",
			"owner", "repo",
		},
		{
			"single segment returns empty",
			"onlyone",
			"", "",
		},
		{
			"empty",
			"",
			"", "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo := parseRepoURL(tt.url)
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}

func TestConvertGHPR(t *testing.T) {
	raw := &ghPR{
		Number:      42,
		Title:       "Test PR",
		URL:         "https://github.com/owner/repo/pull/42",
		State:       "OPEN",
		HeadRefName: "feature-branch",
		HeadRefOid:  "abc123def456",
		BaseRefName: "main",
		IsDraft:     true,
		Mergeable:   "MERGEABLE",
		Additions:   100,
		Deletions:   50,
		CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		ReviewRequests: []ghRequestedReviewer{
			{TypeName: "User", Login: "alice-reviewer"},
			{TypeName: "Team", Slug: "core-platform"},
		},
		Author: struct {
			Login string `json:"login"`
		}{Login: "alice"},
	}

	pr := convertGHPR(raw, "owner", "repo")

	if pr.Number != 42 {
		t.Errorf("number = %d, want 42", pr.Number)
	}
	if pr.State != "open" {
		t.Errorf("state = %q, want open", pr.State)
	}
	if pr.HeadBranch != "feature-branch" {
		t.Errorf("head_branch = %q, want feature-branch", pr.HeadBranch)
	}
	if pr.HeadSHA != "abc123def456" {
		t.Errorf("head_sha = %q, want abc123def456", pr.HeadSHA)
	}
	if !pr.Draft {
		t.Error("expected draft = true")
	}
	if !pr.Mergeable {
		t.Error("expected mergeable = true")
	}
	if pr.Additions != 100 {
		t.Errorf("additions = %d, want 100", pr.Additions)
	}
	if len(pr.RequestedReviewers) != 2 {
		t.Fatalf("requested reviewers = %d, want 2", len(pr.RequestedReviewers))
	}
	if pr.RequestedReviewers[0].Type != reviewerTypeUser {
		t.Errorf("first reviewer type = %q, want %q", pr.RequestedReviewers[0].Type, reviewerTypeUser)
	}
	if pr.RequestedReviewers[1].Type != reviewerTypeTeam {
		t.Errorf("second reviewer type = %q, want %q", pr.RequestedReviewers[1].Type, reviewerTypeTeam)
	}
	if pr.MergedAt != nil {
		t.Error("expected nil MergedAt")
	}
}

func TestConvertGHPR_Merged(t *testing.T) {
	raw := &ghPR{
		Number:   1,
		State:    "CLOSED",
		MergedAt: "2025-01-10T12:00:00Z",
		Author: struct {
			Login string `json:"login"`
		}{Login: "bob"},
	}

	pr := convertGHPR(raw, "owner", "repo")

	if pr.State != prStateMerged {
		t.Errorf("state = %q, want merged", pr.State)
	}
	if pr.MergedAt == nil {
		t.Error("expected non-nil MergedAt")
	}
}

func TestConvertGHPR_NotMergeable(t *testing.T) {
	raw := &ghPR{
		Number:    1,
		State:     "OPEN",
		Mergeable: "CONFLICTING",
		Author: struct {
			Login string `json:"login"`
		}{Login: "alice"},
	}

	pr := convertGHPR(raw, "owner", "repo")

	if pr.Mergeable {
		t.Error("expected mergeable = false for CONFLICTING")
	}
}

func TestConvertGHPR_MergeStateStatus(t *testing.T) {
	tests := []struct {
		name    string
		rawEnum string
		want    string
	}{
		{"clean", "CLEAN", "clean"},
		{"blocked", "BLOCKED", "blocked"},
		{"dirty", "DIRTY", "dirty"},
		{"behind", "BEHIND", "behind"},
		{"unknown", "UNKNOWN", "unknown"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := &ghPR{
				Number:           1,
				State:            "OPEN",
				MergeStateStatus: tt.rawEnum,
				Author: struct {
					Login string `json:"login"`
				}{Login: "alice"},
			}
			pr := convertGHPR(raw, "owner", "repo")
			if pr.MergeableState != tt.want {
				t.Errorf("MergeableState = %q, want %q", pr.MergeableState, tt.want)
			}
		})
	}
}

func TestConvertGHRequestedReviewers(t *testing.T) {
	raw := []ghRequestedReviewer{
		{TypeName: "User", Login: "alice"},
		{TypeName: "Team", Slug: "my-team"},
		{TypeName: "Team", Name: "fallback-team-name"},
		{TypeName: "User"},
	}

	got := convertGHRequestedReviewers(raw)
	if len(got) != 3 {
		t.Fatalf("requested reviewers = %d, want 3", len(got))
	}
	if got[0] != (RequestedReviewer{Login: "alice", Type: reviewerTypeUser}) {
		t.Errorf("unexpected first reviewer: %#v", got[0])
	}
	if got[1] != (RequestedReviewer{Login: "my-team", Type: reviewerTypeTeam}) {
		t.Errorf("unexpected second reviewer: %#v", got[1])
	}
	if got[2] != (RequestedReviewer{Login: "fallback-team-name", Type: reviewerTypeTeam}) {
		t.Errorf("unexpected third reviewer: %#v", got[2])
	}
}

func TestGHStderrIndicatesRateLimit(t *testing.T) {
	cases := []struct {
		stderr string
		want   bool
	}{
		{"GraphQL: API rate limit already exceeded for user ID 12345.", true},
		{"You have exceeded a secondary rate limit.", true},
		{"abuse detection mechanism triggered", true},
		{"network: connection refused", false},
		{"", false},
	}
	for _, c := range cases {
		if got := ghStderrIndicatesRateLimit(c.stderr); got != c.want {
			t.Errorf("ghStderrIndicatesRateLimit(%q) = %v, want %v", c.stderr, got, c.want)
		}
	}
}

func TestGHClient_InspectRateStderr_MarksGraphQL(t *testing.T) {
	tracker := NewRateTracker(nil, nil)
	c := NewGHClient().WithRateTracker(tracker)
	c.inspectRateStderr([]string{"pr", "view", "1"}, "GraphQL: API rate limit already exceeded")
	if !tracker.IsExhausted(ResourceGraphQL) {
		t.Errorf("expected graphql exhausted")
	}
}

func TestGHClient_InspectRateStderr_MarksSearchForSearchEndpoints(t *testing.T) {
	tracker := NewRateTracker(nil, nil)
	c := NewGHClient().WithRateTracker(tracker)
	c.inspectRateStderr([]string{"api", "search/issues"}, "API rate limit exceeded")
	if !tracker.IsExhausted(ResourceSearch) {
		t.Errorf("expected search exhausted")
	}
}

func TestResourceForGHArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want Resource
	}{
		// REST endpoints under `gh api <path>` go to Core, not GraphQL.
		// This is the regression: previously every gh api failure was
		// attributed to GraphQL unless the path started with search/.
		{"api repos REST", []string{"api", "repos/o/r/pulls/1"}, ResourceCore},
		{"api user REST", []string{"api", "user"}, ResourceCore},
		{"api rate_limit REST", []string{"api", "rate_limit"}, ResourceCore},

		// Documented exceptions still resolve to their dedicated buckets.
		{"api graphql", []string{"api", "graphql"}, ResourceGraphQL},
		{"api search/issues", []string{"api", "search/issues"}, ResourceSearch},
		{"api search/repositories", []string{"api", "search/repositories"}, ResourceSearch},

		// Non-`api` subcommands are GraphQL — gh implements pr/issue/repo
		// against the GraphQL API.
		{"pr view", []string{"pr", "view", "1"}, ResourceGraphQL},
		{"issue list", []string{"issue", "list"}, ResourceGraphQL},
		{"repo view", []string{"repo", "view"}, ResourceGraphQL},

		// Defensive defaults for malformed argv.
		{"empty", nil, ResourceCore},
		{"api only", []string{"api"}, ResourceCore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceForGHArgs(tc.args); got != tc.want {
				t.Errorf("resourceForGHArgs(%v) = %s, want %s", tc.args, got, tc.want)
			}
		})
	}
}

func TestBuildUserReposGHArgs(t *testing.T) {
	cases := []struct {
		name  string
		login string
		query string
		limit int
		wantQ string
		wantP string
	}{
		{
			name:  "empty query",
			login: "alice",
			query: "",
			limit: 20,
			wantQ: "q=user:alice",
			wantP: "per_page=20",
		},
		{
			name:  "with query",
			login: "alice",
			query: "language:go",
			limit: 50,
			wantQ: "q=user:alice language:go",
			wantP: "per_page=50",
		},
		{
			name:  "clamped limit at upper bound",
			login: "bob",
			query: "",
			limit: 100,
			wantQ: "q=user:bob",
			wantP: "per_page=100",
		},
		{
			name:  "default clamp value",
			login: "bob",
			query: "in:name foo",
			limit: 20,
			wantQ: "q=user:bob in:name foo",
			wantP: "per_page=20",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := buildUserReposGHArgs(tc.login, tc.query, tc.limit)
			// Sanity: first two args pin the gh subcommand to `api search/repositories`.
			if len(args) < 2 || args[0] != "api" || args[1] != "search/repositories" {
				t.Fatalf("args prefix = %v, want [api search/repositories ...]", args)
			}
			// Find -f q=... and -f per_page=... values without depending on
			// exact positional layout (so future arg additions don't break the test).
			gotQ, gotP := "", ""
			for i := 0; i < len(args)-1; i++ {
				if args[i] != "-f" {
					continue
				}
				switch {
				case strings.HasPrefix(args[i+1], "q="):
					gotQ = args[i+1]
				case strings.HasPrefix(args[i+1], "per_page="):
					gotP = args[i+1]
				}
			}
			if gotQ != tc.wantQ {
				t.Errorf("q flag = %q, want %q", gotQ, tc.wantQ)
			}
			if gotP != tc.wantP {
				t.Errorf("per_page flag = %q, want %q", gotP, tc.wantP)
			}
		})
	}
}

func TestParseGHSearchRepos(t *testing.T) {
	// Mixed payload: one public repo with a description and a default branch
	// other than "main", one private repo with a `null` description that must
	// decode to an empty string, and one repo with no pushed_at to verify the
	// nil-pointer branch.
	data := `[
		{"full_name":"octocat/hello","owner":{"login":"octocat"},"name":"hello","private":false,"default_branch":"trunk","description":"Hello world","pushed_at":"2025-03-01T10:00:00Z"},
		{"full_name":"octocat/secret","owner":{"login":"octocat"},"name":"secret","private":true,"default_branch":"main","description":null,"pushed_at":"2025-02-01T10:00:00Z"},
		{"full_name":"octocat/new","owner":{"login":"octocat"},"name":"new","private":false,"default_branch":"main"}
	]`
	repos, err := parseGHSearchRepos(data)
	if err != nil {
		t.Fatalf("parseGHSearchRepos: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("len = %d, want 3", len(repos))
	}
	if repos[0].FullName != "octocat/hello" || repos[0].DefaultBranch != "trunk" || repos[0].Description != "Hello world" {
		t.Errorf("repo[0] unexpected: %#v", repos[0])
	}
	if repos[0].PushedAt == nil {
		t.Errorf("repo[0] PushedAt nil, want non-nil")
	}
	if !repos[1].Private || repos[1].DefaultBranch != "main" || repos[1].Description != "" {
		t.Errorf("repo[1] unexpected: %#v", repos[1])
	}
	if repos[2].DefaultBranch != "main" || repos[2].PushedAt != nil {
		t.Errorf("repo[2] unexpected: %#v", repos[2])
	}
}

func TestBuildUserReposGHArgs_LimitClamping(t *testing.T) {
	// Mirrors the PAT client test: ListUserRepos must clamp before calling
	// buildUserReposGHArgs, so verify a full round-trip via clampRepoSearchLimit.
	cases := []struct {
		name        string
		inLimit     int
		wantPerPage string
	}{
		{"zero defaults to 20", 0, "per_page=20"},
		{"negative defaults to 20", -5, "per_page=20"},
		{"in range passes through", 42, "per_page=42"},
		{"exceeds cap clamps to 100", 500, "per_page=100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := buildUserReposGHArgs("alice", "", clampRepoSearchLimit(tc.inLimit))
			gotP := ""
			for i := 0; i < len(args)-1; i++ {
				if args[i] == "-f" && strings.HasPrefix(args[i+1], "per_page=") {
					gotP = args[i+1]
				}
			}
			if gotP != tc.wantPerPage {
				t.Errorf("per_page flag = %q, want %q", gotP, tc.wantPerPage)
			}
		})
	}
}

// Regression: a 429 on `gh api repos/...` (REST) must mark Core, not GraphQL.
// Previously this would pause the GraphQL PR monitor incorrectly.
func TestGHClient_InspectRateStderr_RestEndpointMarksCore(t *testing.T) {
	tracker := NewRateTracker(nil, nil)
	c := NewGHClient().WithRateTracker(tracker)
	c.inspectRateStderr([]string{"api", "repos/o/r/pulls/1"}, "API rate limit exceeded")
	if !tracker.IsExhausted(ResourceCore) {
		t.Errorf("expected core bucket exhausted for REST endpoint")
	}
	if tracker.IsExhausted(ResourceGraphQL) {
		t.Errorf("REST 429 must not pause graphql bucket")
	}
}

func TestDecodeGHCheckRunsReadsPaginatedJSONStream(t *testing.T) {
	got, err := decodeGHCheckRuns(`{"name":"page one","status":"completed","conclusion":"success"}
{"name":"page two","status":"completed","conclusion":"failure"}`)
	if err != nil {
		t.Fatalf("decodeGHCheckRuns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("check runs = %d, want 2", len(got))
	}
	if got[1].Name != "page two" || got[1].Conclusion == nil || *got[1].Conclusion != "failure" {
		t.Fatalf("second check run not decoded: %#v", got[1])
	}
}

func TestBuildAccessibleReposGHArgs(t *testing.T) {
	cases := []struct {
		name     string
		inLimit  int
		wantPath string
	}{
		{"in range", 50, "/user/repos?affiliation=owner,collaborator,organization_member&sort=pushed&per_page=50"},
		{"cap clamps to 100", 100, "/user/repos?affiliation=owner,collaborator,organization_member&sort=pushed&per_page=100"},
		{"small page", 5, "/user/repos?affiliation=owner,collaborator,organization_member&sort=pushed&per_page=5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := buildAccessibleReposGHArgs(clampRepoSearchLimit(tc.inLimit))
			if len(args) != 2 || args[0] != "api" {
				t.Fatalf("args = %v, want [api <path>]", args)
			}
			if args[1] != tc.wantPath {
				t.Errorf("path = %q, want %q", args[1], tc.wantPath)
			}
			// Must NOT include --paginate: the picker wants a single page.
			for _, a := range args {
				if a == "--paginate" {
					t.Errorf("args must not contain --paginate, got %v", args)
				}
			}
		})
	}
}

func TestBuildAccessibleReposGHArgs_ClampsPerPage(t *testing.T) {
	cases := []struct {
		name        string
		inLimit     int
		wantPerPage string
	}{
		{"zero defaults to 20", 0, "per_page=20"},
		{"negative defaults to 20", -5, "per_page=20"},
		{"exceeds cap clamps to 100", 500, "per_page=100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := buildAccessibleReposGHArgs(clampRepoSearchLimit(tc.inLimit))
			if !strings.Contains(args[1], tc.wantPerPage) {
				t.Errorf("path %q must contain %q", args[1], tc.wantPerPage)
			}
		})
	}
}

// TestParseGHSearchRepos_FlatArray confirms the flat /user/repos array shape
// (used by ListAccessibleRepos) decodes correctly: happy path, empty, and a
// null description mapping to empty string.
func TestParseGHSearchRepos_FlatArray(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		data := `[
			{"full_name":"kdlbs/kandev","owner":{"login":"kdlbs"},"name":"kandev","private":false,"default_branch":"main","description":"the app","pushed_at":"2025-05-01T10:00:00Z"}
		]`
		repos, err := parseGHSearchRepos(data)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(repos) != 1 || repos[0].FullName != "kdlbs/kandev" || repos[0].Owner != "kdlbs" {
			t.Fatalf("unexpected repos: %#v", repos)
		}
		if repos[0].PushedAt == nil {
			t.Errorf("PushedAt nil, want non-nil")
		}
	})
	t.Run("empty", func(t *testing.T) {
		repos, err := parseGHSearchRepos(`[]`)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(repos) != 0 {
			t.Errorf("len = %d, want 0", len(repos))
		}
	})
	t.Run("null description", func(t *testing.T) {
		data := `[{"full_name":"o/r","owner":{"login":"o"},"name":"r","private":true,"default_branch":"main","description":null}]`
		repos, err := parseGHSearchRepos(data)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if repos[0].Description != "" {
			t.Errorf("description = %q, want empty", repos[0].Description)
		}
		if repos[0].PushedAt != nil {
			t.Errorf("PushedAt = %v, want nil", repos[0].PushedAt)
		}
	})
}
