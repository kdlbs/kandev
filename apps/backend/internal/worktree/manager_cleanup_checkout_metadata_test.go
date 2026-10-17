package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureCleanupHeadOIDs_RelativeCheckoutPath(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runGit(t, repoPath, "worktree", "add", linked, "feature/pr-branch")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, checkout := range []string{repoPath, linked} {
		t.Run(filepath.Base(checkout), func(t *testing.T) {
			relative, err := filepath.Rel(cwd, checkout)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSpace(runGit(t, checkout, "rev-parse", "HEAD"))
			got, err := newCaptureTestManager(t).CaptureCleanupHeadOIDs(context.Background(), []*Worktree{{
				ID: "relative-checkout", RepositoryPath: repoPath, Path: relative, Branch: "feature/pr-branch",
			}})
			if err != nil {
				t.Fatalf("capture relative checkout identity: %v", err)
			}
			if got["relative-checkout"] != want {
				t.Fatalf("identity = %q, want %q", got["relative-checkout"], want)
			}
		})
	}
}
