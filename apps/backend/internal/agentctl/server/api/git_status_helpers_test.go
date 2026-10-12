package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := manager.StopForTeardown(ctx); err != nil {
			t.Errorf("stop multi-repo status fixture: %v", err)
		}
	})
	return NewServer(cfg, manager, nil, nil, log), repoNames
}

func TestMultiRepoStatusFixtureStopsRepositoryTrackers(t *testing.T) {
	var manager *process.Manager
	var trackers []*process.WorkspaceTracker
	t.Run("fixture", func(t *testing.T) {
		server, repos := newMultiRepoStatusServer(t)
		manager = server.procMgr
		for _, repo := range repos {
			tracker, err := manager.GetWorkspaceTrackerFor(repo)
			if err != nil {
				t.Fatal(err)
			}
			trackers = append(trackers, tracker)
		}
	})
	if manager == nil {
		t.Fatal("fixture did not create a manager")
	}
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	_, release, err := manager.BeginOwnedOperation(context.Background())
	if release != nil {
		release()
	}
	if !errors.Is(err, process.ErrManagerStopping) {
		t.Errorf("fixture left process admission open: %v", err)
	}
	for _, tracker := range trackers {
		if _, err := tracker.GetGitStatus(context.Background(), true); !errors.Is(err, context.Canceled) {
			t.Errorf("fixture left a repository tracker live: %v", err)
		}
	}
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
