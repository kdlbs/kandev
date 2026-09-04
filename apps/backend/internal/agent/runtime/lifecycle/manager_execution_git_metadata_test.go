package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrepareExecutionGitMetadataAddsLazyWorktreeProjection(t *testing.T) {
	repo := t.TempDir()
	runLazyGit(t, "", "init", "-b", "main", repo)
	runLazyGit(t, repo, "config", "user.email", "test@example.com")
	runLazyGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	runLazyGit(t, repo, "add", "file")
	runLazyGit(t, repo, "commit", "-m", "initial")
	workspaceRoot := filepath.Join(t.TempDir(), "task")
	workspace := filepath.Join(workspaceRoot, "repo")
	runLazyGit(t, repo, "worktree", "add", "-b", "task", workspace)

	info := &WorkspaceInfo{
		WorkspacePath: workspaceRoot,
		WorkspaceRepositories: []WorkspaceRepositorySpec{{
			RepositoryPath: repo,
			RepoName:       "repo",
			BranchSlug:     "",
		}},
	}
	req := &ExecutorCreateRequest{}
	if err := (&Manager{}).prepareExecutionGitMetadata(info, &mockStopTracker{}, req); err != nil {
		t.Fatalf("prepareExecutionGitMetadata() error = %v", err)
	}
	if len(req.GitMetadataProjections) != 1 {
		t.Fatalf("projections = %d, want 1", len(req.GitMetadataProjections))
	}
	if req.GitMetadataProjections[0].CheckoutPath != workspace {
		t.Fatalf("checkout path = %q, want %q", req.GitMetadataProjections[0].CheckoutPath, workspace)
	}
}

func TestPrepareExecutionGitMetadataMarksLazyCloneRequirement(t *testing.T) {
	info := &WorkspaceInfo{WorkspaceRepositories: []WorkspaceRepositorySpec{{RepositoryPath: "/source", RepoName: "repo"}}}
	req := &ExecutorCreateRequest{}
	runtime := &lazyCloneExecutor{}
	if err := (&Manager{}).prepareExecutionGitMetadata(info, runtime, req); err != nil {
		t.Fatalf("prepareExecutionGitMetadata() error = %v", err)
	}
	if !req.GitMetadataRequirement.RequiresMutableCloneCheckout() {
		t.Fatal("lazy clone request did not require Git metadata attestation")
	}
}

type lazyCloneExecutor struct{ mockStopTracker }

func (*lazyCloneExecutor) RequiresCloneURL() bool { return true }

func runLazyGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	if output, err := exec.CommandContext(context.Background(), "git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
