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
	"github.com/kandev/kandev/internal/common/logger"
)

func TestMultiRepoReviewEndpointsUseStoredBaseBranches(t *testing.T) {
	taskRoot := t.TempDir()
	bases := map[string]string{"frontend": "develop", "backend": "release"}
	baseCommits := make(map[string]string, len(bases))
	baseCommit := ""
	for repo, baseBranch := range bases {
		repoDir := filepath.Join(taskRoot, repo)
		if err := os.Mkdir(repoDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		runGitAPI(t, repoDir, "init", "--initial-branch="+baseBranch)
		runGitAPI(t, repoDir, "config", "user.email", "test@test.com")
		runGitAPI(t, repoDir, "config", "user.name", "Test User")
		writeFileAPI(t, repoDir, "README.md", "base\n")
		runGitAPI(t, repoDir, "add", ".")
		runGitAPI(t, repoDir, "commit", "-m", "initial")
		repoBaseCommit := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))
		baseCommits[repo] = repoBaseCommit
		if baseCommit == "" {
			baseCommit = repoBaseCommit
		}
		runGitAPI(t, repoDir, "checkout", "-b", "feature/review")
		writeFileAPI(t, repoDir, "changed.txt", repo+" change\n")
		runGitAPI(t, repoDir, "add", ".")
		runGitAPI(t, repoDir, "commit", "-m", "feature change")
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: taskRoot, BaseBranches: bases}
	mgr := process.NewManager(cfg, log)
	srv := NewServer(cfg, mgr, nil, nil, log)

	logResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		logResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/git/log?limit=100", nil),
	)
	if logResponse.Code != http.StatusOK {
		t.Fatalf("git log status = %d: %s", logResponse.Code, logResponse.Body.String())
	}
	var commits process.GitLogResult
	if err := json.Unmarshal(logResponse.Body.Bytes(), &commits); err != nil {
		t.Fatalf("decode git log: %v", err)
	}
	commitsByRepo := make(map[string]int)
	for _, commit := range commits.Commits {
		commitsByRepo[commit.RepositoryName]++
	}
	for repo := range bases {
		if commitsByRepo[repo] != 1 {
			t.Fatalf("commits for %s = %d, want 1: %s", repo, commitsByRepo[repo], logResponse.Body.String())
		}
	}

	diffResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		diffResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/git/cumulative-diff?base="+baseCommit, nil),
	)
	if diffResponse.Code != http.StatusOK {
		t.Fatalf("cumulative diff status = %d: %s", diffResponse.Code, diffResponse.Body.String())
	}
	var diff process.CumulativeDiffResult
	if err := json.Unmarshal(diffResponse.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode cumulative diff: %v", err)
	}
	if len(diff.Files) != len(bases) {
		t.Fatalf("cumulative diff files = %d, want %d: %s", len(diff.Files), len(bases), diffResponse.Body.String())
	}
	for repo := range bases {
		payload, ok := diff.Files[repo+"\x00changed.txt"]
		if !ok {
			t.Errorf("cumulative diff missing %s/changed.txt: %s", repo, diffResponse.Body.String())
			continue
		}
		file, ok := payload.(map[string]interface{})
		if !ok {
			t.Fatalf("cumulative diff payload for %s has type %T", repo, payload)
		}
		if got := file["base_ref"]; got != baseCommits[repo] {
			t.Errorf("cumulative diff base_ref for %s = %v, want %s", repo, got, baseCommits[repo])
		}
	}

	missingRepoResponse := httptest.NewRecorder()
	srv.Router().ServeHTTP(
		missingRepoResponse,
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/workspace/file/content-at-ref?path=README.md&ref=HEAD",
			nil,
		),
	)
	if !strings.Contains(missingRepoResponse.Body.String(), "repo is required for multi-repo workspace") {
		t.Fatalf(
			"repo-less file content response = %d: %s",
			missingRepoResponse.Code,
			missingRepoResponse.Body.String(),
		)
	}

	for repo, baseBranch := range bases {
		contentResponse := httptest.NewRecorder()
		path := "/api/v1/workspace/file/content-at-ref?repo=" + repo +
			"&path=README.md&ref=" + baseBranch
		srv.Router().ServeHTTP(
			contentResponse,
			httptest.NewRequest(http.MethodGet, path, nil),
		)
		if contentResponse.Code != http.StatusOK {
			t.Fatalf(
				"file content at ref for %s status = %d: %s",
				repo,
				contentResponse.Code,
				contentResponse.Body.String(),
			)
		}
		var content struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(contentResponse.Body.Bytes(), &content); err != nil {
			t.Fatalf("decode file content at ref for %s: %v", repo, err)
		}
		if content.Content != "base\n" {
			t.Errorf("file content at ref for %s = %q, want %q", repo, content.Content, "base\n")
		}
	}
}
