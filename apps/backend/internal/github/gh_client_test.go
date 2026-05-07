package github

import (
	"errors"
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
