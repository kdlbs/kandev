package api

import (
	"context"
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

// @covers AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.2, AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.3, AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.10
// This contract test pairs with the lifecycle recovery regression that captures
// the map before CreateInstance; no readiness-time correction is applied here.
func TestRecoveredBaseBranches_FirstGitResponses(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "all configured"
		if mixed {
			name = "mixed configured and fallback"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			bases := map[string]string{"alpha": "origin/develop", "beta": "origin/develop", "gamma": "origin/develop"}
			anchors := make(map[string]string)
			for _, repo := range []string{"alpha", "beta", "gamma"} {
				main, develop := seedRecoveredComparisonRepo(t, filepath.Join(root, repo))
				anchors[repo] = develop
				if mixed && repo == "gamma" {
					delete(bases, repo)
					anchors[repo] = main
				}
			}
			log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			cfg := &config.InstanceConfig{WorkDir: root, BaseBranches: bases}
			mgr := process.NewManager(cfg, log)
			t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
			server := NewServer(cfg, mgr, nil, nil, log)
			want := map[string]string{}
			if mixed {
				want["gamma"] = "integration.txt"
			}
			assertRecoveredGitResponses(t, server, anchors, want)
			repoDir := filepath.Join(root, "alpha")
			writeFileAPI(t, repoDir, "task.txt", "task change\n")
			runGitAPI(t, repoDir, "add", "task.txt")
			runGitAPI(t, repoDir, "commit", "-m", "task commit")
			want["alpha"] = "task.txt"
			assertRecoveredGitResponses(t, server, anchors, want)
		})
	}
}

func seedRecoveredComparisonRepo(t *testing.T, dir string) (string, string) {
	t.Helper()
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.name", "Test User")
	runGitAPI(t, dir, "config", "user.email", "test@test.com")
	writeFileAPI(t, dir, "README.md", "initial\n")
	runGitAPI(t, dir, "add", ".")
	runGitAPI(t, dir, "commit", "-m", "initial")
	main := strings.TrimSpace(runGitAPI(t, dir, "rev-parse", "HEAD"))
	runGitAPI(t, dir, "update-ref", "refs/remotes/origin/main", main)
	runGitAPI(t, dir, "checkout", "-b", "develop")
	writeFileAPI(t, dir, "integration.txt", "integration history\n")
	runGitAPI(t, dir, "add", ".")
	runGitAPI(t, dir, "commit", "-m", "integration commit")
	develop := strings.TrimSpace(runGitAPI(t, dir, "rev-parse", "HEAD"))
	runGitAPI(t, dir, "update-ref", "refs/remotes/origin/develop", develop)
	runGitAPI(t, dir, "checkout", "-b", "feature/task")
	return main, develop
}

func assertRecoveredGitResponses(t *testing.T, server *Server, anchors, want map[string]string) {
	t.Helper()
	var logResult process.GitLogResult
	readRecoveredGitJSON(t, server, "/api/v1/git/log?limit=100", &logResult)
	if !logResult.Success || len(logResult.Commits) != len(want) {
		t.Fatalf("log=%+v, want %d commits", logResult, len(want))
	}
	seen := make(map[string]bool)
	for _, commit := range logResult.Commits {
		if _, ok := want[commit.RepositoryName]; !ok {
			t.Errorf("unexpected history commit from %s", commit.RepositoryName)
		}
		if seen[commit.RepositoryName] {
			t.Errorf("duplicate commit for %s", commit.RepositoryName)
		}
		seen[commit.RepositoryName] = true
		expected := "task commit"
		if commit.RepositoryName == "gamma" {
			expected = "integration commit"
		}
		if commit.CommitMessage != expected {
			t.Errorf("commit message=%q, want %q", commit.CommitMessage, expected)
		}
	}
	var diff process.CumulativeDiffResult
	readRecoveredGitJSON(t, server, "/api/v1/git/cumulative-diff?base="+anchors["alpha"], &diff)
	if !diff.Success || len(diff.Files) != len(want) {
		t.Fatalf("diff=%+v, want %d files", diff, len(want))
	}
	for repo, file := range want {
		payload, ok := diff.Files[repo+"\x00"+file].(map[string]interface{})
		if !ok || payload["base_ref"] != anchors[repo] {
			t.Errorf("diff for %s=%v, want base %s", repo, payload, anchors[repo])
		}
	}
	var status MultiRepoGitStatusResult
	readRecoveredGitJSON(t, server, "/api/v1/git/status/multi?fresh=true", &status)
	if !status.Success || len(status.Repos) != len(anchors) {
		t.Fatalf("status=%+v", status)
	}
	for _, repo := range status.Repos {
		if !repo.Status.Success || repo.Status.StatusState != "ready" || !repo.Status.FilesComplete {
			t.Errorf("initial status for %s=%+v, want complete live membership", repo.RepositoryName, repo.Status)
		}
	}

	var detailed MultiRepoGitStatusResult
	readRecoveredGitJSON(t, server, "/api/v1/git/status/multi?fresh=true&details=wait", &detailed)
	if !detailed.Success || len(detailed.Repos) != len(anchors) {
		t.Fatalf("detailed status=%+v", detailed)
	}
	detailedByRepo := make(map[string]GitStatusResult, len(detailed.Repos))
	for _, repo := range detailed.Repos {
		detailedByRepo[repo.RepositoryName] = repo.Status
	}
	for _, repo := range status.Repos {
		expectedAhead := 0
		if _, ok := want[repo.RepositoryName]; ok {
			expectedAhead = 1
		}
		detailedStatus, ok := detailedByRepo[repo.RepositoryName]
		if !ok {
			t.Errorf("detailed status is missing repository %q", repo.RepositoryName)
			continue
		}
		if !detailedStatus.Success || detailedStatus.DetailState != "ready" || detailedStatus.BaseCommit != anchors[repo.RepositoryName] || detailedStatus.Ahead != expectedAhead {
			t.Errorf("status for %s=%+v, want ready details using base %s and ahead %d", repo.RepositoryName, detailedStatus, anchors[repo.RepositoryName], expectedAhead)
		}
	}
}

func readRecoveredGitJSON(t *testing.T, server *Server, path string, out interface{}) {
	t.Helper()
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
