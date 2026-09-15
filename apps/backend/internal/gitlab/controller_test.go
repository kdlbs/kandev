package gitlab

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// newControllerFixture wires a real PATClient + Controller against an
// httptest.NewServer GitLab stub. The returned *requestLog captures every
// path + query the stub observed so tests can assert exactly which params
// the tab-token translator emitted on the wire.
//
// Most tests only care about the /merge_requests and /issues calls, but
// the stub also satisfies /user so the review_requested path works.
func newControllerFixture(t *testing.T, username string) (*gin.Engine, *requestLog, func()) {
	t.Helper()
	log := newTestLogger(t)
	gin.SetMode(gin.TestMode)

	rec := &requestLog{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"` + username + `"}`))
	})
	collect := func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.Path, r.URL.Query())
		w.Header().Set("X-Total", "0")
		_, _ = w.Write([]byte(`[]`))
	}
	mux.HandleFunc("/api/v4/merge_requests", collect)
	mux.HandleFunc("/api/v4/issues", collect)
	srv := httptest.NewServer(mux)

	client := NewPATClient(srv.URL, "tok")
	svc := NewService(srv.URL, client, AuthMethodPAT, nil, log)
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	secrets := &configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace("workspace-test"): "tok",
	}}
	if err := store.UpsertConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{
		Host: srv.URL, AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed GitLab config: %v", err)
	}
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(secrets)
	ctrl := NewController(svc, log)
	router := gin.New()
	ctrl.RegisterHTTPRoutes(router)
	return router, rec, srv.Close
}

type recordedRequest struct {
	Path  string
	Query url.Values
}

type requestLog struct {
	mu       sync.Mutex
	requests []recordedRequest
}

func (r *requestLog) add(path string, q url.Values) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, recordedRequest{Path: path, Query: q})
}

func (r *requestLog) findByPath(path string) *recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.requests {
		if r.requests[i].Path == path {
			return &r.requests[i]
		}
	}
	return nil
}

// hit issues a GET against the in-process router and returns the response.
func hit(router *gin.Engine, target string) *httptest.ResponseRecorder {
	separator := "?"
	if strings.Contains(target, "?") {
		separator = "&"
	}
	if !strings.Contains(target, "workspace_id=") {
		target += separator + "workspace_id=workspace-test"
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHttpListProjectBranches_UnconfiguredPublicGitLab(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "gitlab.com" {
			t.Fatalf("anonymous request host = %q, want gitlab.com", req.URL.Host)
		}
		if got := req.Header.Get("PRIVATE-TOKEN"); got != "" {
			t.Fatalf("PRIVATE-TOKEN = %q, want absent for anonymous read", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[{"name":"main"}]`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	resp := hit(router, "/api/v1/gitlab/projects/branches?project=group%2Fproject&expected_host=https%3A%2F%2Fgitlab.com")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"name":"main"`) {
		t.Fatalf("response = %s, want main branch", resp.Body.String())
	}
}

func TestHttpListProjectBranches_ConfiguredUpstreamNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(stop)

	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	if err := store.UpsertConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{
		Host: host, AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed GitLab config: %v", err)
	}
	svc := NewService(host, NewNoopClient(host), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace("workspace-test"): "token",
	}})
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)

	resp := hit(router, "/api/v1/gitlab/projects/branches?project=group%2Fmissing&expected_host="+url.QueryEscape(host))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
}

func TestHttpListProjectBranches_UnconfiguredPublicNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	svc.SetStore(store)
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"not found"}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	resp := hit(router, "/api/v1/gitlab/projects/branches?project=group%2Fmissing&expected_host=https%3A%2F%2Fgitlab.com")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
}

// Regression for the /gitlab page tabs: each tab value must reach GitLab
// as a real scoping param. Before the translator was added the bare token
// became an empty-value key (e.g. `assigned_to_me=`) and the page served
// the global, unscoped listing.
func TestHttpSearchUserMRs_TranslatesTabFilters(t *testing.T) {
	cases := []struct {
		name        string
		filter      string
		wantScope   string
		wantExtras  map[string]string
		wantMissing []string
	}{
		{
			name:      "assigned_to_me",
			filter:    "assigned_to_me",
			wantScope: "assigned_to_me",
		},
		{
			name:      "created_by_me",
			filter:    "created_by_me",
			wantScope: "created_by_me",
		},
		{
			name:      "review_requested_resolves_username",
			filter:    "review_requested",
			wantScope: "all",
			wantExtras: map[string]string{
				"reviewer_username": "alice",
			},
		},
		{
			// Power-user passthrough: a raw key=value string must still
			// reach appendFilter unchanged. The default `scope=all` stays
			// because the user didn't override it.
			name:      "raw_key_value_passthrough",
			filter:    "labels=bug",
			wantScope: "all",
			wantExtras: map[string]string{
				"labels": "bug",
			},
			wantMissing: []string{"reviewer_username"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, rec, stop := newControllerFixture(t, "alice")
			defer stop()

			resp := hit(router, "/api/v1/gitlab/user/mrs?filter="+url.QueryEscape(tc.filter))
			if resp.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
			}
			req := rec.findByPath("/api/v4/merge_requests")
			if req == nil {
				t.Fatal("/api/v4/merge_requests was never called")
			}
			if got := req.Query.Get("scope"); got != tc.wantScope {
				t.Errorf("scope = %q, want %q", got, tc.wantScope)
			}
			if got := req.Query.Get("state"); got != "opened" {
				t.Errorf("state = %q, want opened (default seeded by buildMRSearchQuery)", got)
			}
			for k, want := range tc.wantExtras {
				if got := req.Query.Get(k); got != want {
					t.Errorf("query[%q] = %q, want %q", k, got, want)
				}
			}
			for _, k := range tc.wantMissing {
				if got := req.Query.Get(k); got != "" {
					t.Errorf("query[%q] = %q, want absent", k, got)
				}
			}
			// Belt-and-braces: the buggy code path used to leave the raw
			// tab token as an empty-value key — verify it's gone.
			if _, present := req.Query[tc.filter]; present && strings.Contains(tc.filter, "_") {
				t.Errorf("query contains stray empty-value key %q (regression: bare token leaked through)", tc.filter)
			}
		})
	}
}

// custom_query must completely bypass tab translation — the translator
// must never run when the power-user escape hatch is used.
func TestHttpSearchUserMRs_CustomQueryBypassesTranslation(t *testing.T) {
	router, rec, stop := newControllerFixture(t, "alice")
	defer stop()

	resp := hit(router,
		"/api/v1/gitlab/user/mrs?filter=assigned_to_me&custom_query="+
			url.QueryEscape("state=closed&labels=bug"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	req := rec.findByPath("/api/v4/merge_requests")
	if req == nil {
		t.Fatal("/api/v4/merge_requests was never called")
	}
	if got := req.Query.Get("state"); got != "closed" {
		t.Errorf("state = %q, want closed (custom_query wins)", got)
	}
	if got := req.Query.Get("labels"); got != "bug" {
		t.Errorf("labels = %q, want bug", got)
	}
	// Translation must not run: no scope param, no leakage of the filter.
	if got := req.Query.Get("scope"); got != "" {
		t.Errorf("scope = %q, want absent (custom_query bypasses defaults)", got)
	}
}

// Defensive backstop: if /user resolves successfully but yields no
// username (NoopClient, or some GitLab response we didn't model), the
// controller must NOT silently fall through to the unscoped /merge_requests
// listing — that would re-expose the original bug. A 500 surfaces the
// problem so the user knows something is wrong instead of seeing a feed
// of unrelated MRs.
func TestHttpSearchUserMRs_ReviewRequestedWithoutUsername_Returns500(t *testing.T) {
	rec := &requestLog{}
	mux := http.NewServeMux()
	// /user succeeds but returns an empty username.
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":""}`))
	})
	mux.HandleFunc("/api/v4/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.Path, r.URL.Query())
		w.Header().Set("X-Total", "0")
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	svc := NewService(srv.URL, NewPATClient(srv.URL, "tok"), AuthMethodPAT, nil, log)
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-test")
	if err := store.UpsertConfigForWorkspace(t.Context(), "workspace-test", &GitLabConfig{
		Host: srv.URL, AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed GitLab config: %v", err)
	}
	svc.SetStore(store)
	svc.SetWorkspaceSecretStore(&configTestSecrets{values: map[string]string{
		SecretKeyForWorkspace("workspace-test"): "tok",
	}})
	router := gin.New()
	NewController(svc, log).RegisterHTTPRoutes(router)

	resp := hit(router, "/api/v1/gitlab/user/mrs?filter=review_requested")
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", resp.Code, resp.Body.String())
	}
	if req := rec.findByPath("/api/v4/merge_requests"); req != nil {
		t.Errorf("/api/v4/merge_requests was called with query %v — controller should short-circuit instead", req.Query)
	}
}

// review_requested must be rejected with 400 on the issues endpoint — GitLab
// has no reviewer-assigned concept for issues. Accepting it and silently
// falling through to an unscoped listing would re-introduce the same bug
// this PR fixes for MRs.
func TestHttpSearchUserIssues_ReviewRequestedReturns400(t *testing.T) {
	router, rec, stop := newControllerFixture(t, "alice")
	defer stop()

	resp := hit(router, "/api/v1/gitlab/user/issues?filter=review_requested")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", resp.Code, resp.Body.String())
	}
	if req := rec.findByPath("/api/v4/issues"); req != nil {
		t.Errorf("/api/v4/issues was called with query %v — controller should short-circuit", req.Query)
	}
}

// Issues counterpart — same regression guarantee plus confirmation that
// review_requested is explicitly rejected (no equivalent GitLab API concept).
func TestHttpSearchUserIssues_TranslatesTabFilters(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantScope string
	}{
		{"assigned_to_me", "assigned_to_me", "assigned_to_me"},
		{"created_by_me", "created_by_me", "created_by_me"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, rec, stop := newControllerFixture(t, "alice")
			defer stop()

			resp := hit(router, "/api/v1/gitlab/user/issues?filter="+url.QueryEscape(tc.filter))
			if resp.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
			}
			req := rec.findByPath("/api/v4/issues")
			if req == nil {
				t.Fatal("/api/v4/issues was never called")
			}
			if got := req.Query.Get("scope"); got != tc.wantScope {
				t.Errorf("scope = %q, want %q", got, tc.wantScope)
			}
			if got := req.Query.Get("state"); got != "opened" {
				t.Errorf("state = %q, want opened", got)
			}
		})
	}
}

// httpSearchUserIssues wiring is otherwise only unit-tested one layer down
// (buildIssueSearchQuery, trimGitLabWhitespace); nothing previously asserted
// end to end that the ?milestone= query key is actually read off the
// request and reaches the outbound GitLab call. A typo in the query key
// name would have passed every other test in this package.
func TestHttpSearchUserIssues_MilestoneReachesUpstreamRequest(t *testing.T) {
	router, rec, stop := newControllerFixture(t, "alice")
	defer stop()

	resp := hit(router, "/api/v1/gitlab/user/issues?filter=assigned_to_me&milestone="+url.QueryEscape("Next"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	req := rec.findByPath("/api/v4/issues")
	if req == nil {
		t.Fatal("/api/v4/issues was never called")
	}
	if got := req.Query.Get("milestone"); got != "Next" {
		t.Errorf("milestone = %q, want Next", got)
	}
	if got := req.Query.Get("scope"); got != "assigned_to_me" {
		t.Errorf("scope = %q, want assigned_to_me (milestone must not clobber the preset)", got)
	}
}

// A whitespace-only milestone (Scenario 3's second boundary) trims to "" and
// must produce a request identical to omitting the parameter entirely — no
// milestone key on the wire at all.
func TestHttpSearchUserIssues_WhitespaceOnlyMilestoneOmitsKey(t *testing.T) {
	router, rec, stop := newControllerFixture(t, "alice")
	defer stop()

	resp := hit(router, "/api/v1/gitlab/user/issues?filter=assigned_to_me&milestone="+url.QueryEscape("   "))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	req := rec.findByPath("/api/v4/issues")
	if req == nil {
		t.Fatal("/api/v4/issues was never called")
	}
	if req.Query.Has("milestone") {
		t.Errorf("query = %v, want no milestone key for a whitespace-only value", req.Query)
	}
}

func TestHttpSearchUserIssues_MalformedCustomQueryReturns400(t *testing.T) {
	router, rec, stop := newControllerFixture(t, "alice")
	defer stop()

	resp := hit(
		router,
		"/api/v1/gitlab/user/issues?custom_query="+url.QueryEscape("%zz")+
			"&milestone="+url.QueryEscape("Next"),
	)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", resp.Code, resp.Body.String())
	}
	if req := rec.findByPath("/api/v4/issues"); req != nil {
		t.Errorf("/api/v4/issues was called with query %v — malformed custom query should be rejected", req.Query)
	}
}
