package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestHandleFileSearchIncludesEveryTaskRepository(t *testing.T) {
	type result struct {
		RepositoryName string `json:"repository_name"`
		Path           string `json:"path"`
	}
	type response struct {
		Files   []string `json:"files"`
		Results []result `json:"results"`
	}

	taskRoot := t.TempDir()
	for _, fixture := range []struct {
		repository string
		path       string
	}{
		{repository: "backend", path: "src/shared-search.go"},
		{repository: "frontend", path: "src/shared-search.ts"},
	} {
		repoDir := filepath.Join(taskRoot, fixture.repository)
		initWorkspaceGitRepo(t, repoDir)
		fullPath := filepath.Join(repoDir, fixture.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: taskRoot}
	manager := process.NewManager(cfg, log)
	manager.StartAllWorkspaceTrackers(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := manager.Stop(ctx); err != nil {
			t.Errorf("stop manager: %v", err)
		}
	})
	server := NewServer(cfg, manager, nil, nil, log)

	deadline := time.Now().Add(2 * time.Second)
	var files []string
	var results []result
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workspace/search?q=shared", nil)
		resp := httptest.NewRecorder()
		server.router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
		}
		var body response
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		files = body.Files
		results = body.Results
		if len(files) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	want := []string{"backend/src/shared-search.go", "frontend/src/shared-search.ts"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}

	wantResults := []result{
		{RepositoryName: "backend", Path: "backend/src/shared-search.go"},
		{RepositoryName: "frontend", Path: "frontend/src/shared-search.ts"},
	}
	if !reflect.DeepEqual(results, wantResults) {
		t.Fatalf("results = %+v, want %+v", results, wantResults)
	}
}
