package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestConvertPatPR(t *testing.T) {
	raw := &patPR{
		Number:    10,
		Title:     "Feature Y",
		HTMLURL:   "https://github.com/org/repo/pull/10",
		State:     "open",
		Draft:     false,
		Additions: 200,
		Deletions: 30,
		CreatedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
		RequestedReviewers: []struct {
			Login string `json:"login"`
		}{
			{Login: "alice-reviewer"},
		},
		RequestedTeams: []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		}{
			{Slug: "platform-team"},
		},
		User: struct {
			Login string `json:"login"`
		}{Login: "bob"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "feature-y", SHA: "deadbeef1234"},
		Base: struct {
			Ref string `json:"ref"`
		}{Ref: "main"},
	}

	pr := convertPatPR(raw, "org", "repo")

	if pr.Number != 10 {
		t.Errorf("number = %d, want 10", pr.Number)
	}
	if pr.State != "open" {
		t.Errorf("state = %q, want open", pr.State)
	}
	if pr.AuthorLogin != "bob" {
		t.Errorf("author = %q, want bob", pr.AuthorLogin)
	}
	if pr.HeadBranch != "feature-y" {
		t.Errorf("head = %q, want feature-y", pr.HeadBranch)
	}
	if pr.HeadSHA != "deadbeef1234" {
		t.Errorf("head_sha = %q, want deadbeef1234", pr.HeadSHA)
	}
	if pr.Mergeable {
		t.Error("expected mergeable = false when nil")
	}
	if len(pr.RequestedReviewers) != 2 {
		t.Fatalf("requested reviewers = %d, want 2", len(pr.RequestedReviewers))
	}
	if pr.RequestedReviewers[0] != (RequestedReviewer{Login: "alice-reviewer", Type: reviewerTypeUser}) {
		t.Errorf("unexpected first requested reviewer: %#v", pr.RequestedReviewers[0])
	}
	if pr.RequestedReviewers[1] != (RequestedReviewer{Login: "platform-team", Type: reviewerTypeTeam}) {
		t.Errorf("unexpected second requested reviewer: %#v", pr.RequestedReviewers[1])
	}
	if pr.MergedAt != nil {
		t.Error("expected nil MergedAt")
	}
}

func TestConvertPatPR_Merged(t *testing.T) {
	mergedAt := "2025-03-05T10:00:00Z"
	raw := &patPR{
		Number:   5,
		State:    "closed",
		MergedAt: &mergedAt,
		User: struct {
			Login string `json:"login"`
		}{Login: "alice"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "fix"},
		Base: struct {
			Ref string `json:"ref"`
		}{Ref: "main"},
	}

	pr := convertPatPR(raw, "org", "repo")

	if pr.State != prStateMerged {
		t.Errorf("state = %q, want merged", pr.State)
	}
	if pr.MergedAt == nil {
		t.Fatal("expected non-nil MergedAt")
	}
}

func TestPATClient_ListCheckRunsPaginatesCheckRuns(t *testing.T) {
	var requestedPages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/widget/commits/sha/status" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"statuses":[]}`))
			return
		}
		if r.URL.Path != "/repos/acme/widget/commits/sha/check-runs" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
		requestedPages = append(requestedPages, r.URL.Query().Get("page"))
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("per_page = %q, want 100", r.URL.Query().Get("per_page"))
		}
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"check_runs":[{"name":"late failure","status":"completed","conclusion":"failure","html_url":"https://checks/fail"}]}`))
			return
		}
		w.Header().Set("Link", `<https://api.github.com/repos/acme/widget/commits/sha/check-runs?per_page=100&page=2>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"check_runs":[{"name":"new success","status":"completed","conclusion":"success","html_url":"https://checks/ok"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	checks, err := c.ListCheckRuns(context.Background(), "acme", "widget", "sha")
	if err != nil {
		t.Fatalf("ListCheckRuns: %v", err)
	}
	if len(requestedPages) != 2 {
		t.Fatalf("requested pages = %v, want first page and page 2", requestedPages)
	}
	if got := computeOverallCheckStatus(checks); got != "failure" {
		t.Fatalf("overall check status = %q, want failure; checks=%#v", got, checks)
	}
}

func TestConvertPatPR_Mergeable(t *testing.T) {
	mergeable := true
	raw := &patPR{
		Number:    1,
		State:     "open",
		Mergeable: &mergeable,
		User: struct {
			Login string `json:"login"`
		}{Login: "alice"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "b"},
		Base: struct {
			Ref string `json:"ref"`
		}{Ref: "main"},
	}

	pr := convertPatPR(raw, "o", "r")
	if !pr.Mergeable {
		t.Error("expected mergeable = true")
	}
}

func TestConvertPatPR_MergeableState(t *testing.T) {
	raw := &patPR{
		Number:         2,
		State:          "open",
		MergeableState: "CLEAN", // GitHub REST uses lowercase but be defensive
		User: struct {
			Login string `json:"login"`
		}{Login: "alice"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "b"},
		Base: struct {
			Ref string `json:"ref"`
		}{Ref: "main"},
	}

	pr := convertPatPR(raw, "o", "r")
	if pr.MergeableState != "clean" {
		t.Errorf("expected normalized mergeable_state=clean, got %q", pr.MergeableState)
	}
}

func TestPATClient_FindPRByBranch_UsesGraphQLHeadRefName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("unexpected path %q, want /graphql", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if !strings.Contains(body.Query, `pullRequests(first: 2, states: OPEN, headRefName: "feature")`) {
			t.Errorf("query should look up branch by headRefName, got: %s", body.Query)
		}
		if strings.Contains(body.Query, `head=acme:feature`) || strings.Contains(body.Query, `ref(qualifiedName:`) {
			t.Errorf("query should not require a base-repo branch ref, got: %s", body.Query)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"data": {
				"b0": {
					"pullRequests": {
						"nodes": [{
							"number": 12,
							"state": "OPEN", "title": "fork PR", "url": "https://x/12",
							"isDraft": false, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
							"headRefName": "feature", "baseRefName": "main", "headRefOid": "abc123",
							"author": {"login":"alice"},
							"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
							"reviews": {"nodes": []}, "reviewRequests": {"totalCount": 0},
							"commits": {"nodes": []}
						}]
					}
				}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	pr, err := c.FindPRByBranch(context.Background(), "acme", "widget", "feature")
	if err != nil {
		t.Fatalf("FindPRByBranch: %v", err)
	}
	if pr == nil || pr.Number != 12 || pr.HeadBranch != "feature" {
		t.Fatalf("unexpected PR: %#v", pr)
	}
}

func TestConvertPatRequestedReviewers(t *testing.T) {
	raw := &patPR{
		RequestedReviewers: []struct {
			Login string `json:"login"`
		}{
			{Login: "alice"},
			{},
		},
		RequestedTeams: []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		}{
			{Slug: "my-team"},
			{Name: "fallback-team"},
			{},
		},
	}

	got := convertPatRequestedReviewers(raw)
	if len(got) != 3 {
		t.Fatalf("requested reviewers = %d, want 3", len(got))
	}
	if got[0] != (RequestedReviewer{Login: "alice", Type: reviewerTypeUser}) {
		t.Errorf("unexpected first reviewer: %#v", got[0])
	}
	if got[1] != (RequestedReviewer{Login: "my-team", Type: reviewerTypeTeam}) {
		t.Errorf("unexpected second reviewer: %#v", got[1])
	}
	if got[2] != (RequestedReviewer{Login: "fallback-team", Type: reviewerTypeTeam}) {
		t.Errorf("unexpected third reviewer: %#v", got[2])
	}
}

func TestPATClient_RecordsRateHeadersOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4998")
		w.Header().Set("X-RateLimit-Reset", "2000000000")
		w.Header().Set("X-RateLimit-Resource", "core")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	tracker := NewRateTracker(nil, nil)
	c.WithRateTracker(tracker)

	var out struct {
		Login string `json:"login"`
	}
	if err := c.get(context.Background(), "/user", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	snap, ok := tracker.Snapshot(ResourceCore)
	if !ok {
		t.Fatalf("expected core snapshot")
	}
	if snap.Remaining != 4998 || snap.Limit != 5000 {
		t.Fatalf("snap = %+v", snap)
	}
}

// Regression: when a 429 carries valid X-RateLimit-Reset headers, the reset
// time from the headers must win — not the synthetic +1h fallback in
// markRateExhausted. Previously, recordRateHeaders called Record(snap)
// followed by markRateExhausted(time.Time{}), and the second call clobbered
// the real reset with a 1-hour pause that could over-throttle the poller.
func TestPATClient_RateLimit429_PreservesRealReset(t *testing.T) {
	realReset := time.Now().Add(7 * time.Minute).UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(realReset.Unix(), 10))
		w.Header().Set("X-RateLimit-Resource", "core")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	tracker := NewRateTracker(nil, nil)
	c.WithRateTracker(tracker)

	var out struct{}
	if err := c.get(context.Background(), "/repos/o/r/pulls/1", &out); err == nil {
		t.Fatalf("expected error from 429")
	}
	snap, ok := tracker.Snapshot(ResourceCore)
	if !ok {
		t.Fatalf("expected core snapshot")
	}
	if !snap.ResetAt.Equal(realReset) {
		t.Errorf("expected reset_at preserved from headers (%s), got %s (off by %s)",
			realReset, snap.ResetAt, snap.ResetAt.Sub(realReset))
	}
	if !tracker.IsExhausted(ResourceCore) {
		t.Errorf("expected core to be exhausted")
	}
}

// When a 429 has no rate-limit headers, the synthetic 1h fallback still
// applies so the poller pauses instead of hammering the secondary limit.
func TestPATClient_RateLimit429_NoHeaders_UsesSyntheticReset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"abuse detection"}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	tracker := NewRateTracker(nil, nil)
	c.WithRateTracker(tracker)

	var out struct{}
	if err := c.get(context.Background(), "/repos/o/r/pulls/1", &out); err == nil {
		t.Fatalf("expected error from 429")
	}
	if !tracker.IsExhausted(ResourceCore) {
		t.Fatalf("expected core exhausted via synthetic fallback")
	}
}

func TestPATClient_MarksExhaustedFromRateLimitBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No headers — secondary limits sometimes omit them entirely.
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for user."}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	tracker := NewRateTracker(nil, nil)
	c.WithRateTracker(tracker)

	var out struct{}
	if err := c.get(context.Background(), "/repos/o/r/pulls/1", &out); err == nil {
		t.Fatalf("expected error from 403")
	}
	if !tracker.IsExhausted(ResourceCore) {
		t.Fatalf("expected core exhausted from body parse")
	}
}

func TestPATClient_FetchBranchProtection(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		wantHasRule  bool
		wantRequired int
		wantErr      bool
	}{
		{
			name:         "200 with required reviews",
			status:       http.StatusOK,
			body:         `{"required_pull_request_reviews":{"required_approving_review_count":2}}`,
			wantHasRule:  true,
			wantRequired: 2,
		},
		{
			name:        "200 with rule but no required reviews block",
			status:      http.StatusOK,
			body:        `{"required_pull_request_reviews":null}`,
			wantHasRule: true,
		},
		{
			name:        "404 maps to no rule",
			status:      http.StatusNotFound,
			body:        `{"message":"Branch not protected"}`,
			wantHasRule: false,
		},
		{
			name:        "403 (no admin scope) maps to no rule",
			status:      http.StatusForbidden,
			body:        `{"message":"Resource not accessible by integration"}`,
			wantHasRule: false,
		},
		{
			name:    "500 propagates as error",
			status:  http.StatusInternalServerError,
			body:    `{"message":"server error"}`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := "/repos/o/r/branches/main/protection"
				if r.URL.Path != wantPath {
					t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			c := newPATClientPointingAt(t, srv.URL)
			bp, err := c.FetchBranchProtection(context.Background(), "o", "r", "main")
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bp.HasRule != tc.wantHasRule {
				t.Fatalf("HasRule = %v, want %v", bp.HasRule, tc.wantHasRule)
			}
			if bp.RequiredApprovingReviewCount != tc.wantRequired {
				t.Fatalf("RequiredApprovingReviewCount = %d, want %d",
					bp.RequiredApprovingReviewCount, tc.wantRequired)
			}
		})
	}
}

func TestListUserRepos_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"login":"octocat"}`))
		case "/search/repositories":
			q := r.URL.Query().Get("q")
			if !strings.Contains(q, "user:octocat") {
				t.Errorf("q = %q, want to contain user:octocat", q)
			}
			if !strings.Contains(q, "demo") {
				t.Errorf("q = %q, want to contain demo", q)
			}
			if got := r.URL.Query().Get("per_page"); got != "50" {
				t.Errorf("per_page = %q, want 50", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items":[
				{"full_name":"octocat/demo","owner":{"login":"octocat"},"name":"demo","private":false,"default_branch":"main","description":"Public demo"},
				{"full_name":"octocat/demo-private","owner":{"login":"octocat"},"name":"demo-private","private":true,"default_branch":"trunk","description":null}
			]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	repos, err := c.ListUserRepos(context.Background(), "demo", 50)
	if err != nil {
		t.Fatalf("ListUserRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("repos = %d, want 2", len(repos))
	}
	if repos[0].FullName != "octocat/demo" || repos[0].Owner != "octocat" || repos[0].Name != "demo" || repos[0].Private {
		t.Errorf("unexpected first repo: %#v", repos[0])
	}
	if repos[0].DefaultBranch != "main" {
		t.Errorf("first repo default_branch = %q, want main", repos[0].DefaultBranch)
	}
	if repos[0].Description != "Public demo" {
		t.Errorf("first repo description = %q, want Public demo", repos[0].Description)
	}
	if !repos[1].Private {
		t.Errorf("expected second repo private")
	}
	if repos[1].DefaultBranch != "trunk" {
		t.Errorf("second repo default_branch = %q, want trunk", repos[1].DefaultBranch)
	}
	// JSON `null` description must decode to an empty string (omitempty drops it on serialize).
	if repos[1].Description != "" {
		t.Errorf("second repo description = %q, want empty string", repos[1].Description)
	}
}

func TestListUserRepos_EmptyQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"login":"alice"}`))
		case "/search/repositories":
			gotQuery = r.URL.Query().Get("q")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	repos, err := c.ListUserRepos(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("ListUserRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("repos = %d, want 0", len(repos))
	}
	if gotQuery != "user:alice" {
		t.Errorf("q = %q, want exactly %q", gotQuery, "user:alice")
	}
}

func TestListUserRepos_LimitClamping(t *testing.T) {
	cases := []struct {
		name        string
		inLimit     int
		wantPerPage string
	}{
		{"zero defaults to 20", 0, "20"},
		{"negative defaults to 20", -5, "20"},
		{"in range passes through", 42, "42"},
		{"exceeds cap clamps to 100", 500, "100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPerPage string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/user":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"login":"alice"}`))
				case "/search/repositories":
					gotPerPage = r.URL.Query().Get("per_page")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"items":[]}`))
				default:
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
			}))
			t.Cleanup(srv.Close)
			c := newPATClientPointingAt(t, srv.URL)
			if _, err := c.ListUserRepos(context.Background(), "", tc.inLimit); err != nil {
				t.Fatalf("ListUserRepos: %v", err)
			}
			if gotPerPage != tc.wantPerPage {
				t.Errorf("per_page = %q, want %q", gotPerPage, tc.wantPerPage)
			}
		})
	}
}

func TestListUserRepos_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"login":"alice"}`))
		case "/search/repositories":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	repos, err := c.ListUserRepos(context.Background(), "", 0)
	if err == nil {
		t.Fatal("expected error from 500, got nil")
	}
	if repos != nil {
		t.Errorf("expected nil repos on error, got %v", repos)
	}
}

func TestListUserRepos_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad creds"}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	repos, err := c.ListUserRepos(context.Background(), "", 0)
	if err == nil {
		t.Fatal("expected error when /user returns 401")
	}
	if repos != nil {
		t.Errorf("expected nil repos on auth error, got %v", repos)
	}
}

func TestClampRepoSearchLimit(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero", 0, 20},
		{"negative", -1, 20},
		{"small", 10, 10},
		{"exact cap", 100, 100},
		{"over cap", 1000, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampRepoSearchLimit(tc.in); got != tc.want {
				t.Errorf("clampRepoSearchLimit(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// newPATClientPointingAt builds a PATClient whose underlying HTTP client
// reroutes any github API URL to the given test server.
func newPATClientPointingAt(t *testing.T, baseURL string) *PATClient {
	t.Helper()
	c := NewPATClient("test-token")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{base: baseURL},
		Timeout:   2 * time.Second,
	}
	return c
}

type rewriteTransport struct{ base string }

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rewritten := strings.Replace(req.URL.String(), githubAPIBase, rt.base, 1)
	req2, err := http.NewRequestWithContext(req.Context(), req.Method, rewritten, req.Body)
	if err != nil {
		return nil, err
	}
	req2.Header = req.Header.Clone()
	return http.DefaultTransport.RoundTrip(req2)
}

func TestListAccessibleRepos_Success(t *testing.T) {
	var gotAffiliation, gotSort, gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			// t.Fatalf must not run in a handler goroutine (it's a data race);
			// use Errorf+return and let the main goroutine fail the assertion.
			t.Errorf("unexpected path %q", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
		gotAffiliation = r.URL.Query().Get("affiliation")
		gotSort = r.URL.Query().Get("sort")
		gotPerPage = r.URL.Query().Get("per_page")
		w.WriteHeader(http.StatusOK)
		// Flat array (NOT a search wrapper with .items).
		_, _ = w.Write([]byte(`[
			{"full_name":"kdlbs/kandev","owner":{"login":"kdlbs"},"name":"kandev","private":false,"default_branch":"main","description":"the app","pushed_at":"2025-05-01T10:00:00Z"},
			{"full_name":"alice/secret","owner":{"login":"alice"},"name":"secret","private":true,"default_branch":"trunk","description":null}
		]`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	repos, err := c.ListAccessibleRepos(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if gotAffiliation != "owner,collaborator,organization_member" {
		t.Errorf("affiliation = %q, want owner,collaborator,organization_member", gotAffiliation)
	}
	if gotSort != "pushed" {
		t.Errorf("sort = %q, want pushed", gotSort)
	}
	if gotPerPage != "50" {
		t.Errorf("per_page = %q, want 50", gotPerPage)
	}
	if len(repos) != 2 {
		t.Fatalf("repos = %d, want 2", len(repos))
	}
	if repos[0].FullName != "kdlbs/kandev" || repos[0].Owner != "kdlbs" || repos[0].DefaultBranch != "main" {
		t.Errorf("unexpected first repo: %#v", repos[0])
	}
	if repos[0].Description != "the app" {
		t.Errorf("first repo description = %q, want 'the app'", repos[0].Description)
	}
	if repos[0].PushedAt == nil {
		t.Errorf("first repo PushedAt nil, want non-nil")
	}
	if !repos[1].Private || repos[1].DefaultBranch != "trunk" {
		t.Errorf("unexpected second repo: %#v", repos[1])
	}
	// JSON null description must decode to empty string.
	if repos[1].Description != "" {
		t.Errorf("second repo description = %q, want empty", repos[1].Description)
	}
}

func TestListAccessibleRepos_QueryFilterAndClamp(t *testing.T) {
	var gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
		gotPerPage = r.URL.Query().Get("per_page")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"full_name":"acme/widget","owner":{"login":"acme"},"name":"widget"},
			{"full_name":"acme/gadget","owner":{"login":"acme"},"name":"gadget"},
			{"full_name":"u/other","owner":{"login":"u"},"name":"other"}
		]`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	// limit above cap must clamp per_page to 100; query filters client-side.
	repos, err := c.ListAccessibleRepos(context.Background(), "WIDGET", 5000)
	if err != nil {
		t.Fatalf("ListAccessibleRepos: %v", err)
	}
	if gotPerPage != "100" {
		t.Errorf("per_page = %q, want 100 (clamped)", gotPerPage)
	}
	if len(repos) != 1 || repos[0].FullName != "acme/widget" {
		t.Fatalf("got %v, want [acme/widget] (case-insensitive substring on full_name)", repos)
	}
}

func TestListAccessibleRepos_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad creds"}`))
	}))
	t.Cleanup(srv.Close)

	c := newPATClientPointingAt(t, srv.URL)
	repos, err := c.ListAccessibleRepos(context.Background(), "", 0)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if repos != nil {
		t.Errorf("expected nil repos on error, got %v", repos)
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("err = %v, want *GitHubAPIError with 401", err)
	}
}
