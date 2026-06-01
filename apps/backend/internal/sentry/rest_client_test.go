package sentry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

// pointTo rewrites the endpoint on a freshly-built client so tests hit the
// httptest server without needing a mockable URL on the production constructor.
func pointTo(c *RESTClient, url string) *RESTClient {
	c.endpoint = url
	return c
}

func TestRESTClient_TestAuth_BearerHeaderAndOK(t *testing.T) {
	var gotAuth string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/" {
			t.Errorf("expected probe to hit /, got %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"user":{"id":"42","username":"alice","name":"Alice","email":"a@x"}}`))
	})
	c := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)
	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("test auth: %v", err)
	}
	if !res.OK || res.DisplayName != "Alice" || res.UserID != "42" {
		t.Errorf("unexpected result: %+v", res)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header = %q, want Bearer tok", gotAuth)
	}
}

func TestRESTClient_TestAuth_Unauthorized(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid token"}`))
	})
	c := pointTo(NewRESTClient(&SentryConfig{}, "bad"), ts.URL)
	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.OK {
		t.Error("expected OK=false")
	}
	if !strings.Contains(res.Error, "401") {
		t.Errorf("expected 401 in error, got %q", res.Error)
	}
}

// TestRESTClient_ListProjects locks in that projects are gathered per-org via
// the org-scoped endpoint (not the user-scoped /projects/, which hides projects
// from org owners not on any team). It also covers the OrgSlug fallback: the
// second org's project node omits the nested organization object, so OrgSlug
// must come from the queried org slug.
func TestRESTClient_ListProjects(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/organizations/":
			_, _ = w.Write([]byte(`[{"id":"10","slug":"acme","name":"Acme"},{"id":"11","slug":"globex","name":"Globex"}]`))
		case "/organizations/acme/projects/":
			_, _ = w.Write([]byte(`[{"id":"1","slug":"frontend","name":"Frontend","organization":{"slug":"acme","name":"Acme"}}]`))
		case "/organizations/globex/projects/":
			// No nested organization object — exercises the OrgSlug fallback.
			_, _ = w.Write([]byte(`[{"id":"2","slug":"api","name":"API"}]`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	c := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)
	projects, err := c.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %+v", projects)
	}
	if projects[0].Slug != "frontend" || projects[0].OrgSlug != "acme" {
		t.Errorf("project[0] = %+v", projects[0])
	}
	if projects[1].Slug != "api" || projects[1].OrgSlug != "globex" {
		t.Errorf("project[1] OrgSlug fallback failed: %+v", projects[1])
	}
}

func TestRESTClient_SearchIssues_BuildsQueryStringAndPaginates(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/projects/acme/frontend/issues/") {
			t.Errorf("expected /projects/acme/frontend/issues/, got %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Has("project") {
			t.Errorf("project slug must be in the path, not the ?project= param: %q", q.Get("project"))
		}
		if q.Get("environment") != "prod" {
			t.Errorf("expected environment=prod, got %q", q.Get("environment"))
		}
		if q.Get("cursor") != "abc" {
			t.Errorf("expected cursor=abc, got %q", q.Get("cursor"))
		}
		got := q.Get("query")
		if !strings.Contains(got, "level:error") || !strings.Contains(got, "is:unresolved") ||
			!strings.Contains(got, "boom") {
			t.Errorf("query string missing tokens: %q", got)
		}
		w.Header().Set("Link", `<https://sentry.io/api/0/x/?cursor=prev>; rel="previous"; results="false"; cursor="prev", `+
			`<https://sentry.io/api/0/x/?cursor=next>; rel="next"; results="true"; cursor="next-cursor"`)
		_, _ = w.Write([]byte(`[{"id":"i1","shortId":"PROJ-1","title":"Boom","level":"error","status":"unresolved","count":"5","userCount":2,"project":{"slug":"frontend","name":"FE"},"assignedTo":{"name":"Alice"}}]`))
	})
	c := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)
	res, err := c.SearchIssues(context.Background(), SearchFilter{
		OrgSlug:     "acme",
		ProjectSlug: "frontend",
		Environment: "prod",
		Levels:      []string{"error"},
		Statuses:    []string{"unresolved"},
		Query:       "boom",
	}, "abc")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.IsLast {
		t.Error("expected IsLast=false when next results=true")
	}
	if res.NextPageToken != "next-cursor" {
		t.Errorf("expected next cursor parsed, got %q", res.NextPageToken)
	}
	if len(res.Issues) != 1 || res.Issues[0].ShortID != "PROJ-1" || res.Issues[0].AssigneeName != "Alice" {
		t.Errorf("issues = %+v", res.Issues)
	}
}

func TestRESTClient_SearchIssues_LastPage(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", `<https://sentry.io/api/0/x/>; rel="next"; results="false"; cursor="x"`)
		_, _ = w.Write([]byte(`[]`))
	})
	c := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)
	res, err := c.SearchIssues(context.Background(), SearchFilter{OrgSlug: "acme"}, "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !res.IsLast {
		t.Error("expected IsLast=true when results=false")
	}
	if res.NextPageToken != "" {
		t.Errorf("expected empty cursor on last page, got %q", res.NextPageToken)
	}
}

func TestRESTClient_SearchIssues_RequiresOrgSlug(t *testing.T) {
	c := pointTo(NewRESTClient(&SentryConfig{}, "tok"), "http://nope")
	_, err := c.SearchIssues(context.Background(), SearchFilter{}, "")
	if err == nil {
		t.Error("expected error when orgSlug missing")
	}
}

// TestRESTClient_SearchIssues_AllProjectsUsesOrgEndpoint locks in that an
// empty project slug falls back to the org-scoped endpoint (browse "all
// projects"), while a set slug uses the project-scoped path (asserted in
// TestRESTClient_SearchIssues_BuildsQueryStringAndPaginates). Regression
// guard for the slug-vs-numeric-id bug: the org endpoint must never receive
// a slug in ?project=.
func TestRESTClient_SearchIssues_AllProjectsUsesOrgEndpoint(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/acme/issues/" {
			t.Errorf("expected /organizations/acme/issues/, got %q", r.URL.Path)
		}
		if r.URL.Query().Has("project") {
			t.Errorf("org endpoint must not receive a project slug param")
		}
		_, _ = w.Write([]byte(`[]`))
	})
	c := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)
	if _, err := c.SearchIssues(context.Background(), SearchFilter{OrgSlug: "acme"}, ""); err != nil {
		t.Fatalf("search: %v", err)
	}
}

func TestRESTClient_GetIssue(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/issues/PROJ-7/" {
			t.Errorf("expected /issues/PROJ-7/, got %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"99","shortId":"PROJ-7","title":"Crash","level":"fatal","status":"unresolved","project":{"slug":"frontend"}}`))
	})
	c := pointTo(NewRESTClient(&SentryConfig{}, "tok"), ts.URL)
	issue, err := c.GetIssue(context.Background(), "PROJ-7")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if issue.ID != "99" || issue.ShortID != "PROJ-7" || issue.Level != "fatal" {
		t.Errorf("issue = %+v", issue)
	}
}

func TestParseNextCursor(t *testing.T) {
	link := `<https://sentry.io/api/0/x/?cursor=prev>; rel="previous"; results="false"; cursor="prev", ` +
		`<https://sentry.io/api/0/x/?cursor=next>; rel="next"; results="true"; cursor="abc-123"`
	cur, has := parseNextCursor(link)
	if !has || cur != "abc-123" {
		t.Errorf("expected abc-123/true, got %q/%v", cur, has)
	}

	// results="false" → no next page.
	link = `<...>; rel="next"; results="false"; cursor="zz"`
	cur, has = parseNextCursor(link)
	if has || cur != "" {
		t.Errorf("expected no-next, got %q/%v", cur, has)
	}

	if _, has := parseNextCursor(""); has {
		t.Error("expected false for empty header")
	}
}

func TestBuildIssueQueryString(t *testing.T) {
	got := buildIssueQueryString(SearchFilter{})
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	got = buildIssueQueryString(SearchFilter{
		Levels:   []string{"error", "fatal"},
		Statuses: []string{"unresolved"},
		Query:    "boom",
	})
	for _, want := range []string{"level:error", "level:fatal", "is:unresolved", "boom"} {
		if !strings.Contains(got, want) {
			t.Errorf("query string %q missing %q", got, want)
		}
	}
}
