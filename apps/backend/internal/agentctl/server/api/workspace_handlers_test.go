package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

// newWorkspaceFileServer builds a server over a plain (non-git) workspace
// seeded with one file and one nested directory. File handlers never touch
// git, so keeping the fixture git-free makes the assertions about the
// filesystem alone.
func newWorkspaceFileServer(t *testing.T) (*Server, string) {
	t.Helper()
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "one.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("seed one.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "nested", "two.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("seed nested/two.txt: %v", err)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: workDir}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log), workDir
}

func workspaceRequest(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	switch v := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(v))
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func decodeWorkspaceBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var body T
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body
}

// TestHandleFileTree_ReturnsRequestedDepth pins the depth query parameter:
// depth=1 must stop at the directory entry, depth=2 must descend into it. A
// handler that ignored the parameter would still return a plausible tree.
func TestHandleFileTree_ReturnsRequestedDepth(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	shallow := workspaceRequest(t, srv, http.MethodGet, "/api/v1/workspace/tree?depth=1", nil)
	if shallow.Code != http.StatusOK {
		t.Fatalf("depth=1 status = %d (body %s)", shallow.Code, shallow.Body.String())
	}
	root := decodeWorkspaceBody[streams.FileTreeResponse](t, shallow).Root
	if root == nil {
		t.Fatalf("depth=1 returned no root: %s", shallow.Body.String())
	}
	nested := findTreeChild(root, "nested")
	if nested == nil {
		t.Fatalf("depth=1 tree has no `nested` entry: %+v", childNames(root))
	}
	if len(nested.Children) != 0 {
		t.Errorf("depth=1 descended into `nested`: %+v", childNames(nested))
	}

	deep := workspaceRequest(t, srv, http.MethodGet, "/api/v1/workspace/tree?depth=2", nil)
	if deep.Code != http.StatusOK {
		t.Fatalf("depth=2 status = %d (body %s)", deep.Code, deep.Body.String())
	}
	deepNested := findTreeChild(decodeWorkspaceBody[streams.FileTreeResponse](t, deep).Root, "nested")
	if deepNested == nil || findTreeChild(deepNested, "two.txt") == nil {
		t.Errorf("depth=2 did not descend into `nested`: %+v", deepNested)
	}
}

// TestHandleFileTree_RejectsEscapingPath covers the 400 branch: the tracker
// refuses a path outside the workspace and the handler forwards the reason.
func TestHandleFileTree_RejectsEscapingPath(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	rec := workspaceRequest(t, srv, http.MethodGet, "/api/v1/workspace/tree?path=../..", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if got := decodeWorkspaceBody[streams.FileTreeResponse](t, rec); got.Error == "" {
		t.Errorf("error is empty; the refusal reason must reach the caller")
	}
}

func findTreeChild(node *streams.FileTreeNode, name string) *streams.FileTreeNode {
	if node == nil {
		return nil
	}
	for _, child := range node.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}

func childNames(node *streams.FileTreeNode) []string {
	names := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		names = append(names, child.Name)
	}
	return names
}

// TestHandleFileContent_ReturnsFileBytes asserts the content endpoint answers
// with the file's real bytes and size, and echoes the caller's repo-relative
// path rather than the resolved absolute one.
func TestHandleFileContent_ReturnsFileBytes(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	rec := workspaceRequest(t, srv, http.MethodGet, "/api/v1/workspace/file/content?path=nested/two.txt", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	got := decodeWorkspaceBody[streams.FileContentResponse](t, rec)
	if got.Content != "two\n" {
		t.Errorf("content = %q, want %q", got.Content, "two\n")
	}
	if got.Path != "nested/two.txt" {
		t.Errorf("path = %q, want the requested repo-relative path", got.Path)
	}
	if got.Size != int64(len("two\n")) {
		t.Errorf("size = %d, want %d", got.Size, len("two\n"))
	}
	if got.IsBinary {
		t.Errorf("is_binary = true for a text file")
	}
}

// TestHandleFileContent_Rejections covers validation and error-status branches
// of the read path.
func TestHandleFileContent_Rejections(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	cases := []struct {
		name       string
		query      string
		wantStatus int
		wantError  string
	}{
		{"missing path", "", http.StatusBadRequest, "path is required"},
		{"unresolvable repo", "?path=one.txt&repo=nope", http.StatusBadRequest, "repo subpath not found"},
		{"missing file", "?path=absent.txt", http.StatusNotFound, "no such file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := workspaceRequest(t, srv, http.MethodGet, "/api/v1/workspace/file/content"+tc.query, nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			got := decodeWorkspaceBody[streams.FileContentResponse](t, rec)
			if !strings.Contains(got.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", got.Error, tc.wantError)
			}
		})
	}
}

// TestHandleFileUpdate_AppliesDiff asserts the patch actually lands on disk and
// that the returned hash is the hash of the new content, which is what the
// editor sends back as OriginalHash on the next save.
func TestHandleFileUpdate_AppliesDiff(t *testing.T) {
	srv, workDir := newWorkspaceFileServer(t)

	rec := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/content", streams.FileUpdateRequest{
		Path: "one.txt",
		Diff: "--- one.txt\n+++ one.txt\n@@ -1 +1 @@\n-one\n+patched\n",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
	if !got.Success {
		t.Fatalf("update failed: %+v", got)
	}
	if got.NewHash == "" {
		t.Errorf("new_hash is empty; the editor needs it for the next conflict check")
	}
	content, err := os.ReadFile(filepath.Join(workDir, "one.txt"))
	if err != nil {
		t.Fatalf("read one.txt: %v", err)
	}
	if string(content) != "patched\n" {
		t.Errorf("one.txt = %q, want %q", string(content), "patched\n")
	}
}

// TestHandleFileUpdate_ReportsHashConflict covers the optimistic-concurrency
// branch: a stale OriginalHash with no desired-content fallback must be
// refused rather than silently overwriting a concurrent edit.
func TestHandleFileUpdate_ReportsHashConflict(t *testing.T) {
	srv, workDir := newWorkspaceFileServer(t)

	rec := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/content", streams.FileUpdateRequest{
		Path:         "one.txt",
		Diff:         "--- one.txt\n+++ one.txt\n@@ -1 +1 @@\n-one\n+patched\n",
		OriginalHash: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
	if got.Success {
		t.Errorf("success = true, want false")
	}
	if !strings.Contains(got.Error, "conflict detected") {
		t.Errorf("error = %q, want a conflict report", got.Error)
	}
	content, err := os.ReadFile(filepath.Join(workDir, "one.txt"))
	if err != nil {
		t.Fatalf("read one.txt: %v", err)
	}
	if string(content) != "one\n" {
		t.Errorf("one.txt = %q; the refused update must not have been written", string(content))
	}
}

// TestHandleFileUpdate_Rejections covers the pre-flight guards, which run
// before anything reads the file.
func TestHandleFileUpdate_Rejections(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	cases := []struct {
		name      string
		body      any
		wantError string
	}{
		{"malformed body", "{not json", "invalid request: "},
		{"missing path", streams.FileUpdateRequest{Diff: "d"}, "path is required"},
		{"missing diff", streams.FileUpdateRequest{Path: "one.txt"}, "diff is required"},
		{
			"unresolvable repo",
			streams.FileUpdateRequest{Path: "one.txt", Diff: "d", Repo: "nope"},
			"repo subpath not found",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/content", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			got := decodeWorkspaceBody[streams.FileUpdateResponse](t, rec)
			if !strings.Contains(got.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", got.Error, tc.wantError)
			}
		})
	}
}

// TestHandleFileCreate_CreatesEmptyFile asserts the created file exists and is
// empty, and that creating over an existing path is refused.
func TestHandleFileCreate_CreatesEmptyFile(t *testing.T) {
	srv, workDir := newWorkspaceFileServer(t)

	rec := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/create", streams.FileCreateRequest{
		Path: "nested/created.txt",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if got := decodeWorkspaceBody[streams.FileCreateResponse](t, rec); !got.Success || got.Path != "nested/created.txt" {
		t.Fatalf("create response = %+v", got)
	}
	info, err := os.Stat(filepath.Join(workDir, "nested", "created.txt"))
	if err != nil {
		t.Fatalf("stat created file: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("created file size = %d, want 0", info.Size())
	}

	again := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/create", streams.FileCreateRequest{
		Path: "one.txt",
	})
	if again.Code != http.StatusBadRequest {
		t.Fatalf("re-create status = %d, want 400 (body %s)", again.Code, again.Body.String())
	}
	if got := decodeWorkspaceBody[streams.FileCreateResponse](t, again); got.Success || got.Error == "" {
		t.Errorf("re-create response = %+v, want a refusal with a reason", got)
	}
}

// TestHandleFileCreate_Rejections covers the guards ahead of the filesystem.
func TestHandleFileCreate_Rejections(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	cases := []struct {
		name      string
		body      any
		wantError string
	}{
		{"malformed body", "{not json", "invalid request: "},
		{"missing path", streams.FileCreateRequest{}, "path is required"},
		{"unresolvable repo", streams.FileCreateRequest{Path: "x.txt", Repo: "nope"}, "repo subpath not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/create", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			got := decodeWorkspaceBody[streams.FileCreateResponse](t, rec)
			if !strings.Contains(got.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", got.Error, tc.wantError)
			}
		})
	}
}

// TestHandleFileDelete_RemovesFile asserts the file is gone from disk, and
// that a second delete of the same path is refused rather than reported as a
// success the UI would render as a removed row.
func TestHandleFileDelete_RemovesFile(t *testing.T) {
	srv, workDir := newWorkspaceFileServer(t)

	rec := workspaceRequest(t, srv, http.MethodDelete, "/api/v1/workspace/file?path=nested/two.txt", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if got := decodeWorkspaceBody[streams.FileDeleteResponse](t, rec); !got.Success || got.Path != "nested/two.txt" {
		t.Fatalf("delete response = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(workDir, "nested", "two.txt")); !os.IsNotExist(err) {
		t.Errorf("nested/two.txt still exists (stat err = %v)", err)
	}

	again := workspaceRequest(t, srv, http.MethodDelete, "/api/v1/workspace/file?path=nested/two.txt", nil)
	if again.Code != http.StatusBadRequest {
		t.Errorf("second delete status = %d, want 400 (body %s)", again.Code, again.Body.String())
	}
}

// TestHandleFileDelete_Rejections covers the query-parameter guards.
func TestHandleFileDelete_Rejections(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	cases := []struct {
		name      string
		query     string
		wantError string
	}{
		{"missing path", "", "path is required"},
		{"unresolvable repo", "?path=one.txt&repo=nope", "repo subpath not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := workspaceRequest(t, srv, http.MethodDelete, "/api/v1/workspace/file"+tc.query, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			got := decodeWorkspaceBody[streams.FileDeleteResponse](t, rec)
			if !strings.Contains(got.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", got.Error, tc.wantError)
			}
		})
	}
}

// TestHandleFileRename_MovesFile asserts both ends of the move: the old path is
// gone and the new path carries the original bytes.
func TestHandleFileRename_MovesFile(t *testing.T) {
	srv, workDir := newWorkspaceFileServer(t)

	rec := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/rename", streams.FileRenameRequest{
		OldPath: "one.txt",
		NewPath: "nested/renamed.txt",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	got := decodeWorkspaceBody[streams.FileRenameResponse](t, rec)
	if !got.Success || got.OldPath != "one.txt" || got.NewPath != "nested/renamed.txt" {
		t.Fatalf("rename response = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(workDir, "one.txt")); !os.IsNotExist(err) {
		t.Errorf("one.txt still exists after rename (stat err = %v)", err)
	}
	content, err := os.ReadFile(filepath.Join(workDir, "nested", "renamed.txt"))
	if err != nil {
		t.Fatalf("read renamed file: %v", err)
	}
	if string(content) != "one\n" {
		t.Errorf("renamed file = %q, want the original %q", string(content), "one\n")
	}
}

// TestHandleFileRename_Rejections covers each guard, including the two
// separate repo-scoping joins the handler performs (old and new path).
func TestHandleFileRename_Rejections(t *testing.T) {
	srv, _ := newWorkspaceFileServer(t)

	cases := []struct {
		name      string
		body      any
		wantError string
	}{
		{"malformed body", "{not json", "invalid request: "},
		{"missing old path", streams.FileRenameRequest{NewPath: "b.txt"}, "old_path is required"},
		{"missing new path", streams.FileRenameRequest{OldPath: "one.txt"}, "new_path is required"},
		{
			"unresolvable repo",
			streams.FileRenameRequest{OldPath: "one.txt", NewPath: "b.txt", Repo: "nope"},
			"repo subpath not found",
		},
		{"missing source", streams.FileRenameRequest{OldPath: "absent.txt", NewPath: "b.txt"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := workspaceRequest(t, srv, http.MethodPost, "/api/v1/workspace/file/rename", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			got := decodeWorkspaceBody[streams.FileRenameResponse](t, rec)
			if got.Success {
				t.Errorf("success = true, want false")
			}
			if got.Error == "" {
				t.Fatalf("error is empty; the refusal reason must reach the caller")
			}
			if !strings.Contains(got.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", got.Error, tc.wantError)
			}
		})
	}
}

// TestMustParseInt covers the query-number helper the tree and search handlers
// share. It answers 0 (not an error) for anything unparseable, which is why
// both callers guard on the result being positive.
func TestMustParseInt(t *testing.T) {
	cases := map[string]int{
		"7":     7,
		"0":     0,
		"-3":    -3,
		"":      0,
		"seven": 0,
		"1.5":   0,
		"9e9":   0,
	}
	for input, want := range cases {
		if got := mustParseInt(input); got != want {
			t.Errorf("mustParseInt(%q) = %d, want %d", input, got, want)
		}
	}
}

// TestHandleFileContentAtRef_ReadsCommittedBytes asserts the endpoint reads
// from git rather than the working tree: the file is edited on disk and the
// response must still carry the committed content for the requested ref.
func TestHandleFileContentAtRef_ReadsCommittedBytes(t *testing.T) {
	fixture := newGitAPIFixture(t)
	writeFileAPI(t, fixture.repo, "one.txt", "dirty working tree\n")

	rec := getGitAPI(t, fixture.server, "/api/v1/workspace/file/content-at-ref?path=one.txt&ref=HEAD")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	got := decodeWorkspaceBody[streams.FileContentResponse](t, rec)
	if got.Content != "one\n" {
		t.Errorf("content = %q, want the committed %q", got.Content, "one\n")
	}
	if got.Path != "one.txt" {
		t.Errorf("path = %q, want %q", got.Path, "one.txt")
	}
}

// TestHandleFileContentAtRef_Rejections covers every guard, including the
// ref that predates the file — a miss inside git, not a bad request shape.
func TestHandleFileContentAtRef_Rejections(t *testing.T) {
	fixture := newGitAPIFixture(t)

	cases := []struct {
		name      string
		query     string
		wantError string
	}{
		{"missing path", "?ref=HEAD", "path is required"},
		{"missing ref", "?path=one.txt", "ref is required"},
		{"unresolvable repo", "?path=one.txt&ref=HEAD&repo=nope", "repo subpath not found"},
		{"ref predating the file", "?path=one.txt&ref=main", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := getGitAPI(t, fixture.server, "/api/v1/workspace/file/content-at-ref"+tc.query)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			got := decodeWorkspaceBody[streams.FileContentResponse](t, rec)
			if got.Error == "" {
				t.Fatalf("error is empty; the refusal reason must reach the caller")
			}
			if !strings.Contains(got.Error, tc.wantError) {
				t.Errorf("error = %q, want it to contain %q", got.Error, tc.wantError)
			}
		})
	}
}

// TestHandleFileContentAtRef_RequiresRepoOnMultiRepoWorkspace pins the
// multi-repo guard. Without an explicit repo the read would land on the task
// root, which is not itself a git repository, and every ref lookup there fails
// for a reason that has nothing to do with the file the user asked for.
func TestHandleFileContentAtRef_RequiresRepoOnMultiRepoWorkspace(t *testing.T) {
	srv, repoNames := newMultiRepoStatusServer(t)

	rec := getGitAPI(t, srv, "/api/v1/workspace/file/content-at-ref?path=README.md&ref=HEAD")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if got := decodeWorkspaceBody[streams.FileContentResponse](t, rec); got.Error != "repo is required for multi-repo workspace" {
		t.Errorf("error = %q, want the multi-repo refusal", got.Error)
	}

	scoped := getGitAPI(t, srv, "/api/v1/workspace/file/content-at-ref?path=README.md&ref=HEAD&repo="+repoNames[0])
	if scoped.Code != http.StatusOK {
		t.Fatalf("scoped status = %d (body %s)", scoped.Code, scoped.Body.String())
	}
	if got := decodeWorkspaceBody[streams.FileContentResponse](t, scoped); got.Content != repoNames[0]+"\n" {
		t.Errorf("content = %q, want %q", got.Content, repoNames[0]+"\n")
	}
}
