package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

func newWorkspacePreviewServer(t *testing.T) (*Server, *process.Manager, string) {
	t.Helper()
	workDir := t.TempDir()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: workDir}
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.CloseWorkspacePreview(context.Background()) })
	return NewServer(cfg, mgr, nil, nil, log), mgr, workDir
}

func publishWorkspacePreview(t *testing.T, srv *Server, repo, path, content string) workspacePreviewPublishResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"repo":    repo,
		"path":    path,
		"content": content,
	})
	if err != nil {
		t.Fatalf("marshal preview request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/html-previews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	var response workspacePreviewPublishResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode preview response (%d): %v; body=%s", rec.Code, err, rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status = %d, response = %+v", rec.Code, response)
	}
	return response
}

func getWorkspacePreview(t *testing.T, response workspacePreviewPublishResponse, path string) (*http.Response, string) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d%s", response.Port, path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", path, err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	return resp, string(body)
}

func TestWorkspacePreviewPublishesOverlayAndServesWorkspaceAssets(t *testing.T) {
	srv, mgr, workDir := newWorkspacePreviewServer(t)
	if err := os.MkdirAll(filepath.Join(workDir, "site", "styles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "site", "styles", "app.css"), []byte("body { color: red; }"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := publishWorkspacePreview(t, srv, "", "site/index.html", "<html><link rel=\"stylesheet\" href=\"styles/app.css\"><body>first</body></html>")
	if first.Port == 0 || first.Path != "/site/index.html" || first.Version != 1 {
		t.Fatalf("first publish = %+v, want port, /site/index.html, version 1", first)
	}

	entry, entryBody := getWorkspacePreview(t, first, first.Path)
	if entry.StatusCode != http.StatusOK {
		t.Fatalf("entry status = %d, want 200", entry.StatusCode)
	}
	if !strings.Contains(entry.Header.Get("Content-Type"), "text/html") {
		t.Errorf("entry content type = %q, want text/html", entry.Header.Get("Content-Type"))
	}
	if entry.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("entry cache control = %q, want no-store", entry.Header.Get("Cache-Control"))
	}
	if !strings.Contains(entryBody, "first") {
		t.Errorf("entry body = %q, want current overlay", entryBody)
	}

	asset, assetBody := getWorkspacePreview(t, first, "/site/styles/app.css")
	if asset.StatusCode != http.StatusOK || assetBody != "body { color: red; }" {
		t.Fatalf("asset = %d %q, want workspace file", asset.StatusCode, assetBody)
	}
	if !strings.Contains(asset.Header.Get("Content-Type"), "text/css") {
		t.Errorf("asset content type = %q, want text/css", asset.Header.Get("Content-Type"))
	}

	second := publishWorkspacePreview(t, srv, "", "site/index.html", "<html><body>second</body></html>")
	if second.Port != first.Port || second.Path != first.Path || second.Version <= first.Version {
		t.Fatalf("replacement = %+v, want same port/path and increasing version after %+v", second, first)
	}
	updated, updatedBody := getWorkspacePreview(t, second, second.Path)
	if updated.StatusCode != http.StatusOK || !strings.Contains(updatedBody, "second") {
		t.Fatalf("updated entry = %d %q, want replacement overlay", updated.StatusCode, updatedBody)
	}

	if err := mgr.StopForTeardown(t.Context()); err != nil {
		t.Fatalf("stop manager: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", first.Port, first.Path), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&http.Client{}).Do(request); err == nil {
		t.Fatal("preview server still accepted requests after manager teardown")
	}
}

func TestWorkspacePreviewRejectsEscapingPathsAndSymlinkTargets(t *testing.T) {
	srv, _, workDir := newWorkspacePreviewServer(t)
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(workDir, "linked.txt")); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"../secret.html", "site/../../secret.html", `%2e%2e/secret.html`} {
		body, err := json.Marshal(map[string]string{"path": path, "content": "<p>blocked</p>"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/html-previews", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q status = %d, want 400", path, rec.Code)
		}
	}

	preview := publishWorkspacePreview(t, srv, "", "index.html", "<script src=\"linked.txt\"></script>")
	linked, linkedBody := getWorkspacePreview(t, preview, "/linked.txt")
	if linked.StatusCode == http.StatusOK || linkedBody == "secret" {
		t.Fatalf("symlink target escaped workspace: status=%d body=%q", linked.StatusCode, linkedBody)
	}
}

func TestWorkspacePreviewEvictsLeastRecentlyPublishedOverlay(t *testing.T) {
	srv, _, _ := newWorkspacePreviewServer(t)
	responses := make([]workspacePreviewPublishResponse, 0, 33)
	for i := 0; i < 33; i++ {
		responses = append(responses, publishWorkspacePreview(
			t,
			srv,
			"",
			fmt.Sprintf("overlay-%02d.html", i),
			fmt.Sprintf("<p>%02d</p>", i),
		))
	}

	oldest, _ := getWorkspacePreview(t, responses[0], responses[0].Path)
	if oldest.StatusCode != http.StatusNotFound {
		t.Fatalf("oldest overlay status = %d, want 404 after 33 publishes", oldest.StatusCode)
	}
	newest, newestBody := getWorkspacePreview(t, responses[32], responses[32].Path)
	if newest.StatusCode != http.StatusOK || !strings.Contains(newestBody, "32") {
		t.Fatalf("newest overlay = %d %q, want retained content", newest.StatusCode, newestBody)
	}
}

func TestWorkspacePreviewRejectsOversizedEntryDocument(t *testing.T) {
	srv, _, _ := newWorkspacePreviewServer(t)
	content := strings.Repeat("x", 5*1024*1024+1)
	body, err := json.Marshal(map[string]string{"path": "index.html", "content": content})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/html-previews", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized publish status = %d, want 413 or 400", rec.Code)
	}
}
