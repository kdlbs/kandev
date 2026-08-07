package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDiagnosticsStreamsToOwnerOnlyServerPath(t *testing.T) {
	server, workDir := newCopyFilesTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspace/diagnostics/0123456789abcdef",
		bytes.NewBufferString("zip-bytes"),
	)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	path := filepath.Join(workDir, ".kandev", "diagnostics", "0123456789abcdef.zip")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "zip-bytes" {
		t.Fatalf("content = %q err=%v", content, err)
	}
}

func TestWorkspaceDiagnosticsRejectsInvalidBundleID(t *testing.T) {
	server, _ := newCopyFilesTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspace/diagnostics/..%2Fescape",
		bytes.NewBufferString("zip-bytes"),
	)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest && response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestEnsureDiagnosticsIgnoredResolvesLinkedWorktreeGitDir(t *testing.T) {
	workDir := t.TempDir()
	commonDir := filepath.Join(workDir, "common.git")
	gitDir := filepath.Join(commonDir, "worktrees", "linked")
	if err := os.MkdirAll(filepath.Join(commonDir, "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(workDir, ".git"),
		[]byte("gitdir: common.git/worktrees/linked\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	ensureDiagnosticsIgnored(workDir)

	exclude, err := os.ReadFile(filepath.Join(commonDir, "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(exclude, []byte("/.kandev/")) {
		t.Fatalf("exclude = %q, want diagnostics pattern", exclude)
	}
}
