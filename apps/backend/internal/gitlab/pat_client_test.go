package gitlab

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

// newTestServer wires a handler under /api/v4/* and returns the host URL.
func newTestServer(t *testing.T, handler http.Handler) (host string, teardown func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/api/v4/", http.StripPrefix("/api/v4", handler))
	srv := httptest.NewServer(mux)
	return srv.URL, srv.Close
}

func TestPATClient_GetAuthenticatedUser(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %q, want /user", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "tok" {
			t.Errorf("PRIVATE-TOKEN header = %q, want tok", r.Header.Get("PRIVATE-TOKEN"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"username": "alice"})
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	user, err := c.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if user != "alice" {
		t.Fatalf("user = %q, want alice", user)
	}
	// Cached on second call.
	if _, err := c.GetAuthenticatedUser(context.Background()); err != nil {
		t.Fatalf("cached call err = %v", err)
	}
}

func TestPATClient_GetAuthenticatedUser_AuthFailure(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer stop()

	c := NewPATClient(host, "bad")
	_, err := c.GetAuthenticatedUser(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", apiErr.StatusCode)
	}
}

func TestPATClient_GetMR(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath preserves %2F; r.URL.Path is the decoded form.
		if got, want := r.URL.EscapedPath(), "/projects/group%2Fproject/merge_requests/42"; got != want {
			t.Errorf("escaped path = %q, want %q (URL-encoded slash)", got, want)
		}
		_ = json.NewEncoder(w).Encode(rawMR{
			ID: 1, IID: 42, ProjectID: 7,
			Title: "Add feature", State: "opened",
			SourceBranch: "feat/x", TargetBranch: "main",
			SHA:    "abc123",
			Author: rawUser{Username: "alice"},
			References: struct {
				Full string `json:"full"`
			}{Full: "group/project!42"},
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	mr, err := c.GetMR(context.Background(), "group/project", 42)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if mr.IID != 42 {
		t.Errorf("iid = %d, want 42", mr.IID)
	}
	if mr.State != "open" {
		t.Errorf("state = %q, want open (normalized from opened)", mr.State)
	}
	if mr.HeadBranch != "feat/x" || mr.BaseBranch != "main" {
		t.Errorf("branches = %q/%q, want feat/x/main", mr.HeadBranch, mr.BaseBranch)
	}
	if mr.AuthorUsername != "alice" {
		t.Errorf("author = %q, want alice", mr.AuthorUsername)
	}
}

func TestPATClient_FindMRByBranch_Empty(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	mr, err := c.FindMRByBranch(context.Background(), "g/p", "missing")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if mr != nil {
		t.Fatalf("mr = %+v, want nil", mr)
	}
}

func TestPATClient_CreateMR_DraftPrefix(t *testing.T) {
	var receivedBody map[string]any
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		_ = json.NewEncoder(w).Encode(rawMR{IID: 7, Title: "Draft: feat", State: "opened"})
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	mr, err := c.CreateMR(context.Background(), "g/p", "feat", "main", "feat", "body", true)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if mr.IID != 7 {
		t.Errorf("iid = %d, want 7", mr.IID)
	}
	if got, want := receivedBody["title"], "Draft: feat"; got != want {
		t.Errorf("title sent = %v, want %q", got, want)
	}
}

func TestPATClient_CreateMR_NoDoubleDraftPrefix(t *testing.T) {
	var receivedBody map[string]any
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		_ = json.NewEncoder(w).Encode(rawMR{IID: 8, State: "opened"})
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	if _, err := c.CreateMR(context.Background(), "g/p", "feat", "main", "Draft: already", "", true); err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := receivedBody["title"].(string); strings.Count(got, "Draft:") != 1 {
		t.Errorf("title = %q, expected single Draft: prefix", got)
	}
}

func TestPATClient_ResolveDiscussion_UsesPUT(t *testing.T) {
	called := false
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "resolved=true") {
			t.Errorf("query = %q, want resolved=true", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	if err := c.ResolveMRDiscussion(context.Background(), "g/p", 1, "abc123"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !called {
		t.Fatal("server was not called")
	}
}

func TestPATClient_ListMRDiscussions_ConvertsThread(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{
			"id": "thread-1",
			"notes": [
				{"id":1,"body":"first","resolvable":true,"resolved":false,"author":{"username":"alice"},"created_at":"2026-05-01T00:00:00Z","updated_at":"2026-05-01T00:00:00Z"},
				{"id":2,"body":"reply","author":{"username":"bob"},"created_at":"2026-05-02T00:00:00Z","updated_at":"2026-05-02T00:00:00Z"}
			]
		}]`))
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	d, err := c.ListMRDiscussions(context.Background(), "g/p", 1, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("got %d discussions, want 1", len(d))
	}
	if !d[0].Resolvable {
		t.Error("resolvable = false, want true (taken from first note)")
	}
	if d[0].Resolved {
		t.Error("resolved = true, want false")
	}
	if len(d[0].Notes) != 2 {
		t.Fatalf("notes = %d, want 2", len(d[0].Notes))
	}
	wantUpdate := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	if !d[0].UpdatedAt.Equal(wantUpdate) {
		t.Errorf("updated_at = %v, want %v (latest reply)", d[0].UpdatedAt, wantUpdate)
	}
}

func TestPATClient_ListMRDiscussions_FiltersBySince(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"id":"old","notes":[{"id":1,"created_at":"2026-04-01T00:00:00Z","updated_at":"2026-04-01T00:00:00Z"}]},
			{"id":"new","notes":[{"id":2,"created_at":"2026-05-15T00:00:00Z","updated_at":"2026-05-15T00:00:00Z"}]}
		]`))
	}))
	defer stop()

	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	c := NewPATClient(host, "tok")
	d, err := c.ListMRDiscussions(context.Background(), "g/p", 1, &since)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(d) != 1 || d[0].ID != "new" {
		t.Fatalf("got %#v, want only the 'new' discussion", d)
	}
}

func TestPATClient_SearchMRsPaged_HonoursTotalHeader(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Total", "153")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 1 {
			_, _ = w.Write([]byte(`[{"iid":1,"state":"opened","author":{"username":"a"},"references":{"full":"g/p!1"}}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	page, err := c.SearchMRsPaged(context.Background(), "", "", 1, 25)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if page.TotalCount != 153 {
		t.Errorf("total = %d, want 153", page.TotalCount)
	}
	if len(page.MRs) != 1 {
		t.Errorf("mrs = %d, want 1", len(page.MRs))
	}
}

func TestParseNextLink(t *testing.T) {
	apiBase := "https://gitlab.example.com/api/v4"
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{`<https://gitlab.example.com/api/v4/issues?page=2>; rel="next"`, "/issues?page=2"},
		{`<https://gitlab.example.com/api/v4/issues?page=2>; rel="prev"`, ""},
		{`<https://gitlab.example.com/api/v4/issues?page=3>; rel="next", <https://gitlab.example.com/api/v4/issues?page=10>; rel="last"`, "/issues?page=3"},
	}
	for _, tc := range cases {
		got := parseNextLink(tc.header, apiBase)
		if got != tc.want {
			t.Errorf("parseNextLink(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestNormalizeMRState(t *testing.T) {
	cases := map[string]string{
		"opened": "open",
		"closed": "closed",
		"merged": "merged",
		"locked": "locked",
		"":       "",
	}
	for in, want := range cases {
		if got := normalizeMRState(in); got != want {
			t.Errorf("normalizeMRState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitFullReference(t *testing.T) {
	cases := []struct {
		in            string
		wantNamespace string
		wantPath      string
	}{
		{"group/project!42", "group", "project"},
		{"group/sub/project!7", "group/sub", "project"},
		{"group/project#10", "group", "project"},
		{"noslash", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		ns, p := splitFullReference(tc.in)
		if ns != tc.wantNamespace || p != tc.wantPath {
			t.Errorf("splitFullReference(%q) = (%q, %q), want (%q, %q)", tc.in, ns, p, tc.wantNamespace, tc.wantPath)
		}
	}
}

func TestSummarizePipelines(t *testing.T) {
	cases := []struct {
		name         string
		input        []Pipeline
		state        string
		expectsTotal bool
	}{
		{"empty", nil, "", false},
		{"success", []Pipeline{{Status: "success", JobsTotal: 3, JobsPassing: 3}}, "success", true},
		{"failed", []Pipeline{{Status: "failed"}}, "failure", false},
		{"running", []Pipeline{{Status: "running"}}, "pending", false},
		{"skipped-suppressed", []Pipeline{{Status: "skipped"}}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, total, _ := summarizePipelines(tc.input)
			if state != tc.state {
				t.Errorf("state = %q, want %q", state, tc.state)
			}
			if tc.expectsTotal && total == 0 {
				t.Error("expected jobs total > 0")
			}
		})
	}
}

func TestSummarizeApprovals(t *testing.T) {
	cases := []struct {
		have, required int
		want           string
	}{
		{0, 0, ""},
		{1, 0, "approved"},
		{2, 2, "approved"},
		{1, 2, "pending"},
		{0, 1, "pending"},
	}
	for _, tc := range cases {
		if got := summarizeApprovals(tc.have, tc.required); got != tc.want {
			t.Errorf("summarizeApprovals(%d,%d) = %q, want %q", tc.have, tc.required, got, tc.want)
		}
	}
}

func TestCountDiffLines(t *testing.T) {
	diff := "--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,3 @@\n-old\n+new1\n+new2\n unchanged\n"
	add, del := countDiffLines(diff)
	if add != 2 {
		t.Errorf("additions = %d, want 2", add)
	}
	if del != 1 {
		t.Errorf("deletions = %d, want 1", del)
	}
}

func TestClampSearchPage(t *testing.T) {
	cases := []struct{ inP, inPP, wantP, wantPP int }{
		{0, 0, 1, defaultPageSize},
		{-3, 5, 1, 5},
		{2, 250, 2, maxPageSize},
		{5, 30, 5, 30},
	}
	for _, tc := range cases {
		p, pp := clampSearchPage(tc.inP, tc.inPP)
		if p != tc.wantP || pp != tc.wantPP {
			t.Errorf("clamp(%d,%d) = (%d,%d), want (%d,%d)", tc.inP, tc.inPP, p, pp, tc.wantP, tc.wantPP)
		}
	}
}
