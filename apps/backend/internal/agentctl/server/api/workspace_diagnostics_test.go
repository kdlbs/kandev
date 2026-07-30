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
