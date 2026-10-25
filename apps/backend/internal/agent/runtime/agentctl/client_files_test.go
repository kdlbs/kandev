package client

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

func TestRequestFileTree_BuildsQueryAndDecodesNestedTree(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"request_id":"req-1",
		"root":{
			"name":"src","path":"src","is_dir":true,
			"children":[
				{"name":"main.go","path":"src/main.go","is_dir":false,"size":42},
				{"name":"link","path":"src/link","is_dir":false,"is_symlink":true}
			]
		}
	}`))

	resp, err := newHTTPOnlyClient(srv.URL).RequestFileTree(context.Background(), "src/deep dir", 3)
	if err != nil {
		t.Fatalf("RequestFileTree: %v", err)
	}

	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if got.Path != "/api/v1/workspace/tree" {
		t.Errorf("path = %q, want /api/v1/workspace/tree", got.Path)
	}
	if p := got.Query.Get("path"); p != "src/deep dir" {
		t.Errorf("path query = %q, want %q", p, "src/deep dir")
	}
	if d := got.Query.Get("depth"); d != "3" {
		t.Errorf("depth query = %q, want 3", d)
	}
	// QueryEscape encodes the space as '+', which is what the server must see.
	if !strings.Contains(got.RawQuery, "path=src%2Fdeep+dir") {
		t.Errorf("raw query = %q, want percent-escaped path", got.RawQuery)
	}

	if resp.RequestID != "req-1" {
		t.Errorf("RequestID = %q, want req-1", resp.RequestID)
	}
	if resp.Root == nil {
		t.Fatal("Root = nil, want decoded tree")
	}
	if resp.Root.Name != "src" || resp.Root.Path != "src" || !resp.Root.IsDir {
		t.Errorf("Root = %+v, want src dir node", resp.Root)
	}
	if len(resp.Root.Children) != 2 {
		t.Fatalf("Root.Children = %d, want 2", len(resp.Root.Children))
	}
	child := resp.Root.Children[0]
	if child.Name != "main.go" || child.Path != "src/main.go" || child.IsDir || child.Size != 42 {
		t.Errorf("Children[0] = %+v, want main.go file of 42 bytes", child)
	}
	if link := resp.Root.Children[1]; !link.IsSymlink || link.Name != "link" {
		t.Errorf("Children[1] = %+v, want symlink node", link)
	}
}

func TestRequestFileTree_SurfacesResponseErrorField(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"error":"path escapes workspace"}`))

	_, err := newHTTPOnlyClient(srv.URL).RequestFileTree(context.Background(), "../etc", 1)
	if err == nil {
		t.Fatal("RequestFileTree returned nil error for error-bearing body")
	}
	if !strings.Contains(err.Error(), "file tree error: path escapes workspace") {
		t.Errorf("error = %v, want wrapped file tree error", err)
	}
}

func TestRequestFileTree_RejectsMalformedBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"root":`))

	_, err := newHTTPOnlyClient(srv.URL).RequestFileTree(context.Background(), "src", 1)
	if err == nil {
		t.Fatal("RequestFileTree returned nil error for truncated JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("error = %v, want parse failure", err)
	}
}

func TestRequestFileTree_HonoursContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newHTTPOnlyClient(srv.URL).RequestFileTree(ctx, "src", 1)
	if err == nil {
		t.Fatal("RequestFileTree returned nil error for cancelled context")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("error = %v, want context canceled", err)
	}
}

func TestSearchFiles_BuildsQueryAndDecodesBothResultShapes(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"files":["a/b.go","c/d.go"],
		"results":[
			{"repository_name":"backend","path":"a/b.go"},
			{"path":"c/d.go"}
		]
	}`))

	resp, err := newHTTPOnlyClient(srv.URL).SearchFiles(context.Background(), "b.go&x", 25)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/workspace/search" {
		t.Errorf("request = %s %s, want GET /api/v1/workspace/search", got.Method, got.Path)
	}
	if q := got.Query.Get("q"); q != "b.go&x" {
		t.Errorf("q = %q, want %q — ampersand must be escaped, not split into a new param", q, "b.go&x")
	}
	if l := got.Query.Get("limit"); l != "25" {
		t.Errorf("limit = %q, want 25", l)
	}

	if len(resp.Files) != 2 || resp.Files[0] != "a/b.go" || resp.Files[1] != "c/d.go" {
		t.Errorf("Files = %v, want both legacy paths", resp.Files)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("Results = %d, want 2", len(resp.Results))
	}
	if resp.Results[0].RepositoryName != "backend" || resp.Results[0].Path != "a/b.go" {
		t.Errorf("Results[0] = %+v, want backend/a/b.go", resp.Results[0])
	}
	if resp.Results[1].RepositoryName != "" || resp.Results[1].Path != "c/d.go" {
		t.Errorf("Results[1] = %+v, want untagged c/d.go", resp.Results[1])
	}
}

func TestSearchFiles_RejectsNonOKStatus(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusServiceUnavailable, `{"files":[]}`))

	_, err := newHTTPOnlyClient(srv.URL).SearchFiles(context.Background(), "q", 5)
	if err == nil {
		t.Fatal("SearchFiles returned nil error for 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error = %v, want status 503 named", err)
	}
}

func TestRequestFileContent_OmitsRepoWhenEmptyAndDecodesEveryField(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"request_id":"rc-1","path":"docs/readme.md","content":"aGk=",
		"size":128,"is_binary":true,"resolved_path":"docs/real.md"
	}`))

	resp, err := newHTTPOnlyClient(srv.URL).RequestFileContent(context.Background(), "docs/readme.md", "")
	if err != nil {
		t.Fatalf("RequestFileContent: %v", err)
	}

	if got.Path != "/api/v1/workspace/file/content" {
		t.Errorf("path = %q, want /api/v1/workspace/file/content", got.Path)
	}
	if _, present := got.Query["repo"]; present {
		t.Errorf("repo query present for single-repo call: %q", got.RawQuery)
	}
	if p := got.Query.Get("path"); p != "docs/readme.md" {
		t.Errorf("path query = %q", p)
	}

	if resp.RequestID != "rc-1" {
		t.Errorf("RequestID = %q, want rc-1", resp.RequestID)
	}
	if resp.Path != "docs/readme.md" {
		t.Errorf("Path = %q", resp.Path)
	}
	if resp.Content != "aGk=" {
		t.Errorf("Content = %q, want aGk=", resp.Content)
	}
	if resp.Size != 128 {
		t.Errorf("Size = %d, want 128", resp.Size)
	}
	if !resp.IsBinary {
		t.Error("IsBinary = false, want true")
	}
	if resp.ResolvedPath != "docs/real.md" {
		t.Errorf("ResolvedPath = %q, want docs/real.md", resp.ResolvedPath)
	}
}

func TestRequestFileContent_AppendsEscapedRepoSubpath(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"path":"a.go","content":"x"}`))

	if _, err := newHTTPOnlyClient(srv.URL).RequestFileContent(context.Background(), "a.go", "my repo/sub"); err != nil {
		t.Fatalf("RequestFileContent: %v", err)
	}
	if repo := got.Query.Get("repo"); repo != "my repo/sub" {
		t.Errorf("repo = %q, want %q", repo, "my repo/sub")
	}
}

func TestRequestFileContent_SurfacesResponseErrorField(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusNotFound, `{"error":"file not found"}`))

	_, err := newHTTPOnlyClient(srv.URL).RequestFileContent(context.Background(), "gone.go", "")
	if err == nil || !strings.Contains(err.Error(), "file content error: file not found") {
		t.Fatalf("error = %v, want wrapped file content error", err)
	}
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("error = %v, want ErrFileNotFound", err)
	}
}

func TestRequestFileContent_DoesNotClassifyNonNotFoundStatus(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusBadRequest, `{"error":"file not found: permission denied"}`))

	_, err := newHTTPOnlyClient(srv.URL).RequestFileContent(context.Background(), "private.go", "")
	if err == nil {
		t.Fatal("RequestFileContent returned nil error for error-bearing body")
	}
	if errors.Is(err, ErrFileNotFound) {
		t.Fatalf("error = %v, want non-not-found error", err)
	}
}

func TestRequestFileContentAtRef_SendsPathRefAndRepo(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"request_id":"ref-1","path":"a.go","content":"old","size":3
	}`))

	resp, err := newHTTPOnlyClient(srv.URL).RequestFileContentAtRef(
		context.Background(), "a.go", "origin/main~2", "svc")
	if err != nil {
		t.Fatalf("RequestFileContentAtRef: %v", err)
	}

	if got.Path != "/api/v1/workspace/file/content-at-ref" {
		t.Errorf("path = %q, want /api/v1/workspace/file/content-at-ref", got.Path)
	}
	if ref := got.Query.Get("ref"); ref != "origin/main~2" {
		t.Errorf("ref = %q, want origin/main~2", ref)
	}
	if p := got.Query.Get("path"); p != "a.go" {
		t.Errorf("path = %q, want a.go", p)
	}
	if repo := got.Query.Get("repo"); repo != "svc" {
		t.Errorf("repo = %q, want svc", repo)
	}
	if resp.RequestID != "ref-1" || resp.Content != "old" || resp.Size != 3 {
		t.Errorf("response = %+v, want fully decoded", resp)
	}
}

func TestRequestFileContentAtRef_OmitsRepoWhenEmpty(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"path":"a.go","content":"x"}`))

	if _, err := newHTTPOnlyClient(srv.URL).RequestFileContentAtRef(
		context.Background(), "a.go", "HEAD", ""); err != nil {
		t.Fatalf("RequestFileContentAtRef: %v", err)
	}
	if _, present := got.Query["repo"]; present {
		t.Errorf("repo query present for single-repo call: %q", got.RawQuery)
	}
}

func TestRequestFileContentAtRef_SurfacesResponseErrorField(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"error":"bad revision"}`))

	_, err := newHTTPOnlyClient(srv.URL).RequestFileContentAtRef(context.Background(), "a.go", "nope", "")
	if err == nil || !strings.Contains(err.Error(), "file content at ref error: bad revision") {
		t.Fatalf("error = %v, want wrapped content-at-ref error", err)
	}
}

func TestApplyFileDiff_MarshalsFullRequestAndDecodesResolution(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"request_id":"u-1","path":"a.go","success":true,
		"new_hash":"deadbeef","resolution":"overwritten"
	}`))

	desired := "package main\n"
	resp, err := newHTTPOnlyClient(srv.URL).ApplyFileDiff(
		context.Background(), "a.go", "@@ -1 +1 @@\n-a\n+b\n", "cafe", "svc", &desired)
	if err != nil {
		t.Fatalf("ApplyFileDiff: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/file/content" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/file/content", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent streams.FileUpdateRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Path != "a.go" || sent.Repo != "svc" || sent.OriginalHash != "cafe" {
		t.Errorf("sent = %+v, want path/repo/hash preserved", sent)
	}
	if sent.Diff != "@@ -1 +1 @@\n-a\n+b\n" {
		t.Errorf("sent.Diff = %q", sent.Diff)
	}
	if sent.DesiredContent == nil || *sent.DesiredContent != desired {
		t.Errorf("sent.DesiredContent = %v, want %q", sent.DesiredContent, desired)
	}

	if resp.RequestID != "u-1" || resp.Path != "a.go" || !resp.Success {
		t.Errorf("response = %+v, want successful update", resp)
	}
	if resp.NewHash != "deadbeef" {
		t.Errorf("NewHash = %q, want deadbeef", resp.NewHash)
	}
	if resp.Resolution != "overwritten" {
		t.Errorf("Resolution = %q, want overwritten", resp.Resolution)
	}
}

func TestApplyFileDiff_OmitsNilDesiredContent(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	if _, err := newHTTPOnlyClient(srv.URL).ApplyFileDiff(
		context.Background(), "a.go", "d", "h", "", nil); err != nil {
		t.Fatalf("ApplyFileDiff: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(got.Body, &raw); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if _, present := raw["desired_content"]; present {
		t.Errorf("desired_content sent despite nil pointer: %s", got.Body)
	}
	if _, present := raw["repo"]; present {
		t.Errorf("repo sent despite empty value: %s", got.Body)
	}
}

func TestApplyFileDiff_SurfacesErrorFieldBeforeSuccessCheck(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"success":false,"error":"hash conflict"}`))

	_, err := newHTTPOnlyClient(srv.URL).ApplyFileDiff(context.Background(), "a.go", "d", "h", "", nil)
	if err == nil || !strings.Contains(err.Error(), "file update error: hash conflict") {
		t.Fatalf("error = %v, want wrapped file update error", err)
	}
}

func TestApplyFileDiff_FailsOnUnsuccessfulResponseWithoutError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"success":false}`))

	_, err := newHTTPOnlyClient(srv.URL).ApplyFileDiff(context.Background(), "a.go", "d", "h", "", nil)
	if err == nil || !strings.Contains(err.Error(), "file update failed") {
		t.Fatalf("error = %v, want file update failed", err)
	}
}

func TestCreateFile_PostsPathAndRepoAndDecodesResponse(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"path":"new.go","success":true}`))

	resp, err := newHTTPOnlyClient(srv.URL).CreateFile(context.Background(), "new.go", "svc")
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/file/create" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/file/create", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent streams.FileCreateRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Path != "new.go" || sent.Repo != "svc" {
		t.Errorf("sent = %+v, want new.go in svc", sent)
	}

	if resp.Path != "new.go" || !resp.Success {
		t.Errorf("response = %+v, want created new.go", resp)
	}
}

func TestCreateFile_SurfacesErrorAndUnsuccessfulResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"error field", `{"success":false,"error":"already exists"}`, "file create error: already exists"},
		{"unsuccessful", `{"success":false}`, "file create failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(http.StatusOK, tc.body))
			_, err := newHTTPOnlyClient(srv.URL).CreateFile(context.Background(), "new.go", "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDeleteFile_UsesDeleteVerbWithEscapedQuery(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"request_id":"d-1","path":"old file.go","success":true}`))

	resp, err := newHTTPOnlyClient(srv.URL).DeleteFile(context.Background(), "old file.go", "svc")
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	if got.Method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", got.Method)
	}
	if got.Path != "/api/v1/workspace/file" {
		t.Errorf("path = %q, want /api/v1/workspace/file", got.Path)
	}
	if p := got.Query.Get("path"); p != "old file.go" {
		t.Errorf("path query = %q, want %q", p, "old file.go")
	}
	if repo := got.Query.Get("repo"); repo != "svc" {
		t.Errorf("repo = %q, want svc", repo)
	}
	if len(got.Body) != 0 {
		t.Errorf("DELETE sent a body: %q", got.Body)
	}

	if resp.RequestID != "d-1" || resp.Path != "old file.go" || !resp.Success {
		t.Errorf("response = %+v, want deleted old file.go", resp)
	}
}

func TestDeleteFile_OmitsRepoWhenEmpty(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	if _, err := newHTTPOnlyClient(srv.URL).DeleteFile(context.Background(), "a.go", ""); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, present := got.Query["repo"]; present {
		t.Errorf("repo query present for single-repo call: %q", got.RawQuery)
	}
}

func TestDeleteFile_SurfacesErrorAndUnsuccessfulResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"error field", `{"success":false,"error":"permission denied"}`, "file delete error: permission denied"},
		{"unsuccessful", `{"success":false}`, "file delete failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(http.StatusOK, tc.body))
			_, err := newHTTPOnlyClient(srv.URL).DeleteFile(context.Background(), "a.go", "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRenameFile_PostsBothPathsAndDecodesResponse(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"old_path":"a.go","new_path":"b.go","success":true}`))

	resp, err := newHTTPOnlyClient(srv.URL).RenameFile(context.Background(), "a.go", "b.go", "svc")
	if err != nil {
		t.Fatalf("RenameFile: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/file/rename" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/file/rename", got.Method, got.Path)
	}

	var sent streams.FileRenameRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.OldPath != "a.go" || sent.NewPath != "b.go" || sent.Repo != "svc" {
		t.Errorf("sent = %+v, want a.go -> b.go in svc", sent)
	}

	if resp.OldPath != "a.go" || resp.NewPath != "b.go" || !resp.Success {
		t.Errorf("response = %+v, want renamed a.go -> b.go", resp)
	}
}

func TestRenameFile_SurfacesErrorAndUnsuccessfulResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"error field", `{"success":false,"error":"target exists"}`, "file rename error: target exists"},
		{"unsuccessful", `{"success":false}`, "file rename failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(http.StatusOK, tc.body))
			_, err := newHTTPOnlyClient(srv.URL).RenameFile(context.Background(), "a.go", "b.go", "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMaterializeDiagnosticBundle_StreamsZipToEscapedPath(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"path":"/diag/b.zip","bytes":9}`))

	resp, err := newHTTPOnlyClient(srv.URL).MaterializeDiagnosticBundle(
		context.Background(), "bundle/id 1", strings.NewReader("ZIPBYTES!"))
	if err != nil {
		t.Fatalf("MaterializeDiagnosticBundle: %v", err)
	}

	if got.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.Method)
	}
	// PathEscape (not QueryEscape) — the ID is a path segment, so '/' must be
	// encoded and a space becomes %20 rather than '+'.
	if got.Path != "/api/v1/workspace/diagnostics/bundle/id 1" {
		t.Errorf("decoded path = %q, want the escaped id as one segment", got.Path)
	}
	if got.ContentType != "application/zip" {
		t.Errorf("content-type = %q, want application/zip", got.ContentType)
	}
	if string(got.Body) != "ZIPBYTES!" {
		t.Errorf("body = %q, want the raw source bytes", got.Body)
	}

	if resp.Path != "/diag/b.zip" || resp.Bytes != 9 {
		t.Errorf("response = %+v, want /diag/b.zip of 9 bytes", resp)
	}
}

func TestMaterializeDiagnosticBundle_FailsOnNonOKStatusAndErrorField(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"non-OK status", http.StatusInsufficientStorage, `{"path":"","bytes":0}`},
		{"error field on 200", http.StatusOK, `{"error":"bundle too large"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))
			_, err := newHTTPOnlyClient(srv.URL).MaterializeDiagnosticBundle(
				context.Background(), "b", strings.NewReader("x"))
			if err == nil || !strings.Contains(err.Error(), "diagnostic materialization failed") {
				t.Fatalf("error = %v, want materialization failure", err)
			}
		})
	}
}

func TestCopyFiles_MarshalsEntriesAndDecodesCopiedAndWarnings(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"copied":[".env",".env.local"],"warnings":["skipped secrets/"]}`))

	entries := []copyfiles.Entry{
		{RelPath: ".env", Mode: 0o600, Content: []byte("A=1")},
		{RelPath: ".env.local", Mode: 0o644, Content: []byte("B=2")},
	}
	resp, err := newHTTPOnlyClient(srv.URL).CopyFiles(context.Background(), "svc", entries)
	if err != nil {
		t.Fatalf("CopyFiles: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/workspace/copy-files" {
		t.Errorf("request = %s %s, want POST /api/v1/workspace/copy-files", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent CopyFilesRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.Repo != "svc" {
		t.Errorf("sent.Repo = %q, want svc", sent.Repo)
	}
	if len(sent.Entries) != 2 {
		t.Fatalf("sent.Entries = %d, want 2", len(sent.Entries))
	}
	if sent.Entries[0].RelPath != ".env" || sent.Entries[0].Mode != 0o600 ||
		string(sent.Entries[0].Content) != "A=1" {
		t.Errorf("sent.Entries[0] = %+v, want .env 0600 A=1", sent.Entries[0])
	}
	if sent.Entries[1].RelPath != ".env.local" || string(sent.Entries[1].Content) != "B=2" {
		t.Errorf("sent.Entries[1] = %+v, want .env.local B=2", sent.Entries[1])
	}

	if len(resp.Copied) != 2 || resp.Copied[0] != ".env" || resp.Copied[1] != ".env.local" {
		t.Errorf("Copied = %v, want both entries", resp.Copied)
	}
	if len(resp.Warnings) != 1 || resp.Warnings[0] != "skipped secrets/" {
		t.Errorf("Warnings = %v, want the skip warning", resp.Warnings)
	}
}

// CopyFiles is the one file helper that returns a non-nil response alongside
// its error, so callers can read partial progress out of a failed batch.
func TestCopyFiles_ReturnsPartialResponseWithError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK,
		`{"copied":[".env"],"error":"disk full"}`))

	resp, err := newHTTPOnlyClient(srv.URL).CopyFiles(context.Background(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "copy-files error: disk full") {
		t.Fatalf("error = %v, want wrapped copy-files error", err)
	}
	if resp == nil {
		t.Fatal("response = nil, want the partial result alongside the error")
	}
	if len(resp.Copied) != 1 || resp.Copied[0] != ".env" {
		t.Errorf("Copied = %v, want the one file that made it", resp.Copied)
	}
}

func TestCopyFiles_RejectsMalformedBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `not json`))

	_, err := newHTTPOnlyClient(srv.URL).CopyFiles(context.Background(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("error = %v, want parse failure", err)
	}
}

// Guard the content-type the diagnostics endpoint depends on: agentctl routes
// the raw body straight to disk, so a JSON content-type would be a silent
// protocol break.
func TestMaterializeDiagnosticBundle_ContentTypeIsExactlyZip(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"path":"p","bytes":1}`))

	if _, err := newHTTPOnlyClient(srv.URL).MaterializeDiagnosticBundle(
		context.Background(), "b", strings.NewReader("x")); err != nil {
		t.Fatalf("MaterializeDiagnosticBundle: %v", err)
	}
	mediaType, _, err := mime.ParseMediaType(got.ContentType)
	if err != nil {
		t.Fatalf("parse content-type %q: %v", got.ContentType, err)
	}
	if mediaType != "application/zip" {
		t.Errorf("media type = %q, want application/zip", mediaType)
	}
}
