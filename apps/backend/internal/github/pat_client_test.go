package github

import (
	"context"
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
