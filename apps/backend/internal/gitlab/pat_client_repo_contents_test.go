package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPATClient_ListRepoTree(t *testing.T) {
	var gotPath, gotQuery string
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "a1", "name": "review.yaml", "type": "blob", "path": ".kandev/workflows/review.yaml"},
			{"id": "b2", "name": "nested", "type": "tree", "path": ".kandev/workflows/nested"},
		})
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	entries, err := c.ListRepoTree(context.Background(), "group/project", ".kandev/workflows", "main")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotPath != "/projects/group%2Fproject/repository/tree" {
		t.Errorf("path = %q, want the project ref URL-encoded as one segment", gotPath)
	}
	// ref and path must be forwarded, and the listing must stay non-recursive
	// so sync keeps matching the GitHub behavior of one directory only.
	for _, want := range []string{"ref=main", "path=.kandev%2Fworkflows", "recursive=false"} {
		if !containsQueryPart(gotQuery, want) {
			t.Errorf("query = %q, want it to contain %q", gotQuery, want)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Name != "review.yaml" || entries[0].Type != "blob" {
		t.Errorf("entries[0] = %+v, want the blob", entries[0])
	}
	if entries[0].Path != ".kandev/workflows/review.yaml" {
		t.Errorf("entries[0].Path = %q, want the full repo path", entries[0].Path)
	}
	if entries[1].Type != "tree" {
		t.Errorf("entries[1].Type = %q, want tree", entries[1].Type)
	}
}

// Nested subgroup paths are the reason projectRef exists: a bare PathEscape
// leaves the slash intact and GitLab routes it to the parent, 404ing the
// subgroup project.
func TestPATClient_ListRepoTree_NestedSubgroupPath(t *testing.T) {
	var gotPath string
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	if _, err := c.ListRepoTree(context.Background(), "acme/team/project", ".kandev/workflows", "main"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotPath != "/projects/acme%2Fteam%2Fproject/repository/tree" {
		t.Errorf("path = %q, want every slash encoded", gotPath)
	}
}

// A directory larger than one page must not silently truncate the synced
// workflow set.
func TestPATClient_ListRepoTree_Paginates(t *testing.T) {
	pages := 0
	var srvURL string
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		if pages == 1 {
			w.Header().Set("Link", fmt.Sprintf(
				`<%s/api/v4/projects/g%%2Fp/repository/tree?page=2>; rel="next"`, srvURL))
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "one.yaml", "type": "blob", "path": "one.yaml"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "two.yaml", "type": "blob", "path": "two.yaml"},
		})
	}))
	defer stop()
	srvURL = host

	c := NewPATClient(host, "tok")
	entries, err := c.ListRepoTree(context.Background(), "g/p", "", "main")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if pages != 2 {
		t.Fatalf("pages = %d, want 2 (pagination not followed)", pages)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 across both pages", len(entries))
	}
	if entries[0].Name != "one.yaml" || entries[1].Name != "two.yaml" {
		t.Errorf("entries = %+v, want page order preserved", entries)
	}
}

func TestPATClient_ListRepoTree_NotFound(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	_, err := c.ListRepoTree(context.Background(), "g/p", "missing", "main")
	if err == nil {
		t.Fatal("err = nil, want a not-found error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want an *APIError with status 404", err)
	}
}

func TestPATClient_GetRepoFileContent(t *testing.T) {
	var gotPath, gotQuery string
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("name: Review\nsteps: []\n"))
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	content, err := c.GetRepoFileContent(
		context.Background(), "acme/team/project", ".kandev/workflows/review.yaml", "main")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// The file path is a single :file_path segment and must be fully encoded,
	// slashes and dots included.
	wantPath := "/projects/acme%2Fteam%2Fproject/repository/files/" +
		".kandev%2Fworkflows%2Freview.yaml/raw"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if !containsQueryPart(gotQuery, "ref=main") {
		t.Errorf("query = %q, want ref=main", gotQuery)
	}
	if string(content) != "name: Review\nsteps: []\n" {
		t.Errorf("content = %q, want the raw bytes verbatim", content)
	}
}

func TestPATClient_GetRepoFileContent_Forbidden(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	_, err := c.GetRepoFileContent(context.Background(), "g/p", "a.yaml", "main")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("err = %v, want an *APIError with status 403", err)
	}
}

// A credential-bearing request must never follow a redirect to a different
// origin: net/http strips the standard sensitive headers (Authorization,
// Cookie, ...) on cross-host redirects but would forward the custom
// PRIVATE-TOKEN header unchanged, leaking the PAT to an untrusted host.
func TestPATClient_RejectsCrossHostRedirect(t *testing.T) {
	attackerHit := false
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attackerHit = true
	}))
	defer attacker.Close()

	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/api/v4/projects/g%2Fp/repository/tree", http.StatusFound)
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	_, err := c.ListRepoTree(context.Background(), "g/p", "", "main")
	if err == nil {
		t.Fatal("err = nil, want a cross-host redirect rejection")
	}
	if !strings.Contains(err.Error(), "cross-host redirect") {
		t.Errorf("err = %v, want cross-host redirect context", err)
	}
	if attackerHit {
		t.Fatal("redirect target received a request, leaking the PRIVATE-TOKEN header")
	}
}

// A same-host redirect (e.g. http -> https upgrade or a trailing-slash
// normalization) is expected GitLab behavior and must keep working.
func TestPATClient_FollowsSameHostRedirect(t *testing.T) {
	var gotToken string
	var host string
	var stop func()
	host, stop = newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/projects/g%2Fp/repository/tree" && r.URL.Query().Get("redirect") == "" {
			q := r.URL.Query()
			q.Set("redirect", "1")
			http.Redirect(w, r, host+"/api/v4/projects/g%2Fp/repository/tree?"+q.Encode(), http.StatusFound)
			return
		}
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer stop()

	c := NewPATClient(host, "tok")
	if _, err := c.ListRepoTree(context.Background(), "g/p", "", "main"); err != nil {
		t.Fatalf("err = %v, want same-host redirect followed", err)
	}
	if gotToken != "tok" {
		t.Errorf("PRIVATE-TOKEN on redirected request = %q, want tok", gotToken)
	}
}

func TestNoopClient_RepoContents(t *testing.T) {
	c := NewNoopClient("")
	if _, err := c.ListRepoTree(context.Background(), "g/p", "", "main"); !errors.Is(err, ErrNoClient) {
		t.Errorf("ListRepoTree err = %v, want ErrNoClient", err)
	}
	if _, err := c.GetRepoFileContent(context.Background(), "g/p", "a.yaml", "main"); !errors.Is(err, ErrNoClient) {
		t.Errorf("GetRepoFileContent err = %v, want ErrNoClient", err)
	}
}

func TestMockClient_RepoContents(t *testing.T) {
	c := NewMockClient("")
	c.SeedRepoFile("g/p", "main", ".kandev/workflows/review.yaml", []byte("name: Review\n"))
	c.SeedRepoFile("g/p", "main", ".kandev/workflows/deploy.yml", []byte("name: Deploy\n"))
	c.SeedRepoFile("g/p", "main", "other/ignored.yaml", []byte("name: Nope\n"))

	entries, err := c.ListRepoTree(context.Background(), "g/p", ".kandev/workflows", "main")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want only the two under the seeded dir", len(entries))
	}

	content, err := c.GetRepoFileContent(context.Background(), "g/p", ".kandev/workflows/review.yaml", "main")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if string(content) != "name: Review\n" {
		t.Errorf("content = %q, want the seeded bytes", content)
	}

	if _, err := c.GetRepoFileContent(context.Background(), "g/p", "absent.yaml", "main"); err == nil {
		t.Error("err = nil for an unseeded file, want a not-found error")
	}
}

// containsQueryPart reports whether a raw query string carries the exact
// key=value pair, avoiding substring false positives across parameters.
func containsQueryPart(rawQuery, want string) bool {
	for _, part := range splitQuery(rawQuery) {
		if part == want {
			return true
		}
	}
	return false
}

func splitQuery(rawQuery string) []string {
	var parts []string
	current := ""
	for _, r := range rawQuery {
		if r == '&' {
			parts = append(parts, current)
			current = ""
			continue
		}
		current += string(r)
	}
	return append(parts, current)
}
