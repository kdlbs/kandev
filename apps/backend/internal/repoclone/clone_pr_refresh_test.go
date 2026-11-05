package repoclone

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.2
func TestRefreshWorkspaceRepositoryTracksUpdatedPullRequest(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	checkout := filepath.Join(root, "checkout")
	runGit(t, root, "init", "--bare", "-b", "main", origin)
	runGit(t, root, "clone", origin, seed)
	runGit(t, seed, "config", "user.name", "Test User")
	runGit(t, seed, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "initial")
	runGit(t, seed, "push", "origin", "HEAD:refs/heads/main")
	runGit(t, root, "clone", origin, checkout)
	cloner := NewCloner(Config{}, ProtocolHTTPS, root, logger.Default())
	checkoutHead := strings.TrimSpace(runGit(t, checkout, "rev-parse", "HEAD"))
	for revision := 0; revision < 3; revision++ {
		runGit(t, seed, "commit", "--allow-empty", "-m", fmt.Sprintf("PR revision %d", revision))
		runGit(t, seed, "push", "origin", "HEAD:refs/pull/42/head")
		if err := cloner.refreshWorkspaceRepository(context.Background(), checkout, origin, nil, 42); err != nil {
			t.Fatalf("refresh PR revision %d: %v", revision, err)
		}
		want := strings.TrimSpace(runGit(t, seed, "rev-parse", "HEAD"))
		got := strings.TrimSpace(runGit(t, checkout, "rev-parse", "refs/kandev/pull/42/head"))
		if got != want {
			t.Fatalf("PR revision %d: got %s, want %s", revision, got, want)
		}
		if got := strings.TrimSpace(runGit(t, checkout, "rev-parse", "HEAD")); got != checkoutHead {
			t.Fatalf("PR revision %d: checkout HEAD = %s, want unchanged %s", revision, got, checkoutHead)
		}
	}
}
