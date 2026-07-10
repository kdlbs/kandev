package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

// newCopyFilesTestServer builds an agentctl Server with a real WorkDir on
// disk so the handler can resolve and write into it.
func newCopyFilesTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: workDir}
	procMgr := process.NewManager(cfg, log)
	s := NewServer(cfg, procMgr, nil, nil, log)
	return s, workDir
}

func TestHandleWorkspaceCopyFiles_WritesEntries(t *testing.T) {
	s, workDir := newCopyFilesTestServer(t)

	body, _ := json.Marshal(CopyFilesRequest{
		Entries: []copyfiles.Entry{
			{RelPath: ".env", Mode: 0o600, Content: []byte("X=1")},
			{RelPath: "config/local.yml", Mode: 0o644, Content: []byte("y")},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/copy-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp CopyFilesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Copied) != 2 {
		t.Errorf("copied = %v, want 2 entries", resp.Copied)
	}
	if got, err := os.ReadFile(filepath.Join(workDir, ".env")); err != nil || string(got) != "X=1" {
		t.Errorf(".env content = %q, err = %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(workDir, "config", "local.yml")); err != nil || string(got) != "y" {
		t.Errorf("config/local.yml content = %q, err = %v", got, err)
	}
}

func TestHandleWorkspaceCopyFiles_RejectsPathTraversal(t *testing.T) {
	s, workDir := newCopyFilesTestServer(t)

	body, _ := json.Marshal(CopyFilesRequest{
		Entries: []copyfiles.Entry{
			{RelPath: "../escape.txt", Mode: 0o644, Content: []byte("leak")},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/copy-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (rejection surfaces as warning, not HTTP error), got %d", w.Code)
	}
	var resp CopyFilesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Copied) != 0 {
		t.Errorf("copied = %v, want zero", resp.Copied)
	}
	if len(resp.Warnings) == 0 {
		t.Error("expected warning for traversal entry")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(workDir), "escape.txt")); !os.IsNotExist(err) {
		t.Errorf("escape.txt should NOT exist outside workDir; stat err = %v", err)
	}
}

func TestHandleWorkspaceCopyFiles_SkipsIfExists(t *testing.T) {
	s, workDir := newCopyFilesTestServer(t)

	if err := os.WriteFile(filepath.Join(workDir, ".env"), []byte("PREEXISTING"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body, _ := json.Marshal(CopyFilesRequest{
		Entries: []copyfiles.Entry{{RelPath: ".env", Mode: 0o644, Content: []byte("NEW")}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/copy-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp CopyFilesResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Copied) != 0 {
		t.Errorf("copied = %v, want zero (skip-if-exists)", resp.Copied)
	}
	got, _ := os.ReadFile(filepath.Join(workDir, ".env"))
	if string(got) != "PREEXISTING" {
		t.Errorf("existing file overwritten: %q", got)
	}
}

func TestHandleWorkspaceCopyFiles_EmptyEntries(t *testing.T) {
	s, _ := newCopyFilesTestServer(t)

	body, _ := json.Marshal(CopyFilesRequest{Entries: nil})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/copy-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty entries, got %d", w.Code)
	}
}

func TestHandleWorkspaceCopyFiles_RejectsBadRepoSubpath(t *testing.T) {
	s, _ := newCopyFilesTestServer(t)

	body, _ := json.Marshal(CopyFilesRequest{
		Repo:    "../escape",
		Entries: []copyfiles.Entry{{RelPath: "x", Mode: 0o644, Content: []byte("x")}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/copy-files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad repo subpath, got %d (body: %s)", w.Code, w.Body.String())
	}
}
