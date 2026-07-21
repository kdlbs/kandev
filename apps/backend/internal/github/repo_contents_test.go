package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoContentsPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty is root", in: "", want: ""},
		{name: "dot is root", in: ".", want: ""},
		{name: "simple path", in: "workflows", want: "workflows"},
		{name: "nested path preserves slashes", in: "a/b/c.yaml", want: "a/b/c.yaml"},
		{name: "segment with space is escaped", in: "my dir/file.yaml", want: "my%20dir/file.yaml"},
		{name: "leading and trailing slashes trimmed", in: "/workflows/", want: "workflows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := repoContentsPath(tc.in); got != tc.want {
				t.Fatalf("repoContentsPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPATClient_ListRepoDirectory_DecodesEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/repos/o/r/contents/workflows"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.URL.Query().Get("ref"); got != "main" {
			t.Errorf("ref = %q, want main", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"name":"deploy.yaml","path":"workflows/deploy.yaml","type":"file","sha":"abc123","size":42},
			{"name":"nested","path":"workflows/nested","type":"dir","sha":"def456","size":0}
		]`))
	}))
	t.Cleanup(srv.Close)
	c := newPATClientPointingAt(t, srv.URL)

	entries, err := c.ListRepoDirectory(context.Background(), "o", "r", "workflows", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "deploy.yaml" || entries[0].Type != "file" || entries[0].SHA != "abc123" || entries[0].Size != 42 {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Name != "nested" || entries[1].Type != "dir" {
		t.Fatalf("unexpected second entry: %+v", entries[1])
	}
}

func TestPATClient_ListRepoDirectory_RootOmitsPathSegment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/repos/o/r/contents"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.URL.Query().Get("ref"); got != "" {
			t.Errorf("ref = %q, want empty (omitted)", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	c := newPATClientPointingAt(t, srv.URL)

	entries, err := c.ListRepoDirectory(context.Background(), "o", "r", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestPATClient_ListRepoDirectory_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)
	c := newPATClientPointingAt(t, srv.URL)

	_, err := c.ListRepoDirectory(context.Background(), "o", "r", "missing", "main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *GitHubAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestPATClient_GetRepoFileContent_DecodesBase64(t *testing.T) {
	want := []byte("name: deploy\nsteps:\n  - run: echo hi\n")
	// GitHub wraps base64 content with embedded newlines every 60 chars;
	// simulate that here to prove the decoder strips them.
	encoded := base64.StdEncoding.EncodeToString(want)
	wrapped := encoded[:len(encoded)/2] + "\n" + encoded[len(encoded)/2:] + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/repos/o/r/contents/workflows/deploy.yaml"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		respBody, err := json.Marshal(map[string]string{
			"type":     "file",
			"encoding": "base64",
			"content":  wrapped,
		})
		if err != nil {
			t.Fatalf("marshal response body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	t.Cleanup(srv.Close)
	c := newPATClientPointingAt(t, srv.URL)

	got, err := c.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestPATClient_GetRepoFileContent_NonBase64EncodingErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"file","encoding":"none","content":"raw text"}`))
	}))
	t.Cleanup(srv.Close)
	c := newPATClientPointingAt(t, srv.URL)

	_, err := c.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "main")
	if err == nil {
		t.Fatal("expected error for non-base64 encoding, got nil")
	}
}

func TestPATClient_GetRepoFileContent_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(srv.Close)
	c := newPATClientPointingAt(t, srv.URL)

	_, err := c.GetRepoFileContent(context.Background(), "o", "r", "missing.yaml", "main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *GitHubAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

// --- MockClient repo-file seeding ---

func TestMockClient_SeedRepoFile_GetRepoFileContent_ExactRef(t *testing.T) {
	m := NewMockClient()
	m.SeedRepoFile("o", "r", "main", "workflows/deploy.yaml", []byte("on-main"))
	m.SeedRepoFile("o", "r", "dev", "workflows/deploy.yaml", []byte("on-dev"))

	got, err := m.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "on-main" {
		t.Fatalf("content = %q, want on-main", got)
	}

	got, err = m.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "on-dev" {
		t.Fatalf("content = %q, want on-dev", got)
	}
}

func TestMockClient_SeedRepoFile_WildcardRefMatchesAnyRequestedRef(t *testing.T) {
	m := NewMockClient()
	m.SeedRepoFile("o", "r", "", "workflows/deploy.yaml", []byte("wildcard"))

	for _, ref := range []string{"", "main", "feature/x"} {
		got, err := m.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", ref)
		if err != nil {
			t.Fatalf("ref %q: unexpected error: %v", ref, err)
		}
		if string(got) != "wildcard" {
			t.Fatalf("ref %q: content = %q, want wildcard", ref, got)
		}
	}
}

func TestMockClient_SeedRepoFile_ExactRefPreferredOverWildcard(t *testing.T) {
	m := NewMockClient()
	m.SeedRepoFile("o", "r", "", "workflows/deploy.yaml", []byte("wildcard"))
	m.SeedRepoFile("o", "r", "main", "workflows/deploy.yaml", []byte("exact-main"))

	got, err := m.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "exact-main" {
		t.Fatalf("content = %q, want exact-main (exact ref should win over wildcard)", got)
	}

	// A specific-ref seed must NOT satisfy a different ref, or the default
	// (empty) ref, since only the wildcard seed matches those.
	got, err = m.GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "dev")
	if err != nil {
		t.Fatalf("unexpected error for dev ref: %v", err)
	}
	if string(got) != "wildcard" {
		t.Fatalf("dev ref content = %q, want wildcard", got)
	}
}

func TestMockClient_GetRepoFileContent_404(t *testing.T) {
	m := NewMockClient()
	_, err := m.GetRepoFileContent(context.Background(), "o", "r", "missing.yaml", "main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *GitHubAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Fatalf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestMockClient_ListRepoDirectory_InfersFilesAndSubdirs(t *testing.T) {
	m := NewMockClient()
	m.SeedRepoFile("o", "r", "main", "workflows/deploy.yaml", []byte("a"))
	m.SeedRepoFile("o", "r", "main", "workflows/nested/inner.yaml", []byte("b"))
	m.SeedRepoFile("o", "r", "main", "README.md", []byte("c"))

	entries, err := m.ListRepoDirectory(context.Background(), "o", "r", "", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byName := make(map[string]RepoContentEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 root entries, got %d: %+v", len(entries), entries)
	}
	if got := byName["README.md"]; got.Type != "file" || got.Path != "README.md" {
		t.Fatalf("README.md entry = %+v, want file at README.md", got)
	}
	if got := byName["workflows"]; got.Type != "dir" || got.Path != "workflows" {
		t.Fatalf("workflows entry = %+v, want dir at workflows", got)
	}

	nested, err := m.ListRepoDirectory(context.Background(), "o", "r", "workflows", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nestedByName := make(map[string]RepoContentEntry, len(nested))
	for _, e := range nested {
		nestedByName[e.Name] = e
	}
	if len(nested) != 2 {
		t.Fatalf("expected 2 entries under workflows, got %d: %+v", len(nested), nested)
	}
	if got := nestedByName["deploy.yaml"]; got.Type != "file" || got.Path != "workflows/deploy.yaml" {
		t.Fatalf("deploy.yaml entry = %+v", got)
	}
	if got := nestedByName["nested"]; got.Type != "dir" || got.Path != "workflows/nested" {
		t.Fatalf("nested entry = %+v", got)
	}
}

func TestMockClient_ListRepoDirectory_404OnMissingDir(t *testing.T) {
	m := NewMockClient()
	m.SeedRepoFile("o", "r", "main", "workflows/deploy.yaml", []byte("a"))

	_, err := m.ListRepoDirectory(context.Background(), "o", "r", "does-not-exist", "main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *GitHubAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Fatalf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestMockClient_ListRepoDirectory_RefScoping(t *testing.T) {
	m := NewMockClient()
	m.SeedRepoFile("o", "r", "main", "only-on-main.yaml", []byte("a"))

	_, err := m.ListRepoDirectory(context.Background(), "o", "r", "", "dev")
	if err == nil {
		t.Fatal("expected 404 for a ref with no matching seeded files, got nil")
	}

	entries, err := m.ListRepoDirectory(context.Background(), "o", "r", "", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "only-on-main.yaml" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

// --- GHClient (gh CLI) ---

// installFakeGH writes a fake `gh` shell script onto PATH for the duration of
// the test, logging every invocation's argv to a file the test can inspect.
// script receives the raw argv (via "$*") and stdout/stderr as it sees fit.
func installFakeGH(t *testing.T, script string) (argsLogPath string) {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh-args.log")
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_ARGS_LOG", logPath)
	return logPath
}

func TestGHClient_ListRepoDirectory_DecodesEntries(t *testing.T) {
	logPath := installFakeGH(t, `#!/bin/sh
printf '%s\n' "$*" >> "$GH_ARGS_LOG"
case "$*" in
  *contents/workflows*)
    printf '%s\n' '[{"name":"deploy.yaml","path":"workflows/deploy.yaml","type":"file","sha":"abc123","size":42}]'
    ;;
  *) printf '%s\n' 'gh: HTTP 404: Not Found' >&2; exit 1 ;;
esac
`)

	entries, err := NewGHClient().ListRepoDirectory(context.Background(), "o", "r", "workflows", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "deploy.yaml" || entries[0].Type != "file" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gh args log: %v", err)
	}
	if !strings.Contains(string(logged), "repos/o/r/contents/workflows") {
		t.Fatalf("expected contents endpoint in gh args, got:\n%s", logged)
	}
	if !strings.Contains(string(logged), "-f ref=main") {
		t.Fatalf("expected ref field in gh args, got:\n%s", logged)
	}
}

func TestGHClient_ListRepoDirectory_404(t *testing.T) {
	installFakeGH(t, `#!/bin/sh
printf '%s\n' 'gh: HTTP 404: Not Found' >&2
exit 1
`)

	_, err := NewGHClient().ListRepoDirectory(context.Background(), "o", "r", "missing", "main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *GitHubAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestGHClient_GetRepoFileContent_DecodesBase64(t *testing.T) {
	want := []byte("name: deploy\n")
	encoded := base64.StdEncoding.EncodeToString(want)
	installFakeGH(t, `#!/bin/sh
printf '{"type":"file","encoding":"base64","content":"`+encoded+`"}\n'
`)

	got, err := NewGHClient().GetRepoFileContent(context.Background(), "o", "r", "workflows/deploy.yaml", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestGHClient_GetRepoFileContent_404(t *testing.T) {
	installFakeGH(t, `#!/bin/sh
printf '%s\n' 'gh: HTTP 404: Not Found' >&2
exit 1
`)

	_, err := NewGHClient().GetRepoFileContent(context.Background(), "o", "r", "missing.yaml", "main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *GitHubAPIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}
