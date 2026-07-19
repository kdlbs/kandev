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
		if baseCommit == "" {
			baseCommit = strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "HEAD"))
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
		if _, ok := diff.Files[repo+"\x00changed.txt"]; !ok {
			t.Errorf("cumulative diff missing %s/changed.txt: %s", repo, diffResponse.Body.String())
		}
	}
}
