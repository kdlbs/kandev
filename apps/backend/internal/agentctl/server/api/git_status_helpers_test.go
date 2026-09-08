package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// Shared multi-repo status fixtures. These live in their own file rather than
// alongside the concurrency test that first needed them because that file is
// //go:build !windows (it needs mkfifo to gate git), while nothing here is
// platform-specific — and a Windows `go vet` fails on any untagged test that
// reaches for a symbol defined only in the tagged one.

func newMultiRepoStatusServer(t *testing.T) (*Server, []string) {
	return newMultiRepoStatusServerWithAgentEnv(t, nil)
}

func newMultiRepoStatusServerWithAgentEnv(t *testing.T, agentEnv []string) (*Server, []string) {
	t.Helper()
	taskRoot := t.TempDir()
	repoNames := []string{"alpha", "beta"}
	for _, repo := range repoNames {
		newStatusTestRepo(t, taskRoot, repo)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{
		WorkDir:  taskRoot,
		AgentEnv: append([]string(nil), agentEnv...),
	}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log), repoNames
}

func newStatusTestRepo(t *testing.T, taskRoot, name string) string {
	t.Helper()
	repoDir := filepath.Join(taskRoot, name)
	if err := os.Mkdir(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	runGitAPI(t, repoDir, "init", "--initial-branch=main")
	runGitAPI(t, repoDir, "config", "user.email", "test@test.com")
	runGitAPI(t, repoDir, "config", "user.name", "Test User")
	writeFileAPI(t, repoDir, "README.md", name+"\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "initial")
	return repoDir
}

func assertRepoNames(t *testing.T, got, want []string) {
	t.Helper()
	gotSet := make(map[string]bool, len(got))
	for _, repo := range got {
		gotSet[repo] = true
	}
	for _, repo := range want {
		if !gotSet[repo] {
			t.Errorf("repositories %v missing %q", got, repo)
		}
	}
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
