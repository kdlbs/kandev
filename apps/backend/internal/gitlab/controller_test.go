package gitlab

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

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
