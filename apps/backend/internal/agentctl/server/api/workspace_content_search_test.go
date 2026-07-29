package api

import (
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
)

func TestHandleWorkspaceContentSearchReturnsTypedMatches(t *testing.T) {
	repoDir := t.TempDir()
	initWorkspaceGitRepo(t, repoDir)
	if err := os.WriteFile(
		filepath.Join(repoDir, "hello.txt"),
		[]byte("zero\nhello Needle world\nanother needle\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: repoDir}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspace/content-search?q=needle&limit_per_repo=1",
		nil,
	)
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var body streams.WorkspaceContentSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Results) != 1 {
		t.Fatalf("results = %#v, want one", body.Results)
	}
	match := body.Results[0]
	if match.Path != "hello.txt" || match.Line != 2 || match.Column != 7 {
		t.Fatalf("match = %#v", match)
	}
}

func TestHandleWorkspaceContentSearchRejectsQueryOverMaximum(t *testing.T) {
	server := newTestServer(t)
	query := strings.Repeat("a", process.WorkspaceContentSearchMaxQueryRunes+1)
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/workspace/content-search?q="+query,
		nil,
	)
	resp := httptest.NewRecorder()
	server.router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", resp.Code, resp.Body.String())
	}
}
