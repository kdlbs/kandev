package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveGitMetadata_LinkedWorktreeContainsOnlyOwnedMetadata(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)

	projection, err := ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatalf("ResolveGitMetadata: %v", err)
	}

	if projection.CheckoutPath != checkout {
		t.Fatalf("CheckoutPath = %q, want %q", projection.CheckoutPath, checkout)
	}
	if projection.GitDir == filepath.Join(repo, ".git") {
		t.Fatal("GitDir must be the linked worktree metadata directory, not the source .git")
	}
	if projection.CommonDir != filepath.Join(repo, ".git") {
		t.Fatalf("CommonDir = %q, want %q", projection.CommonDir, filepath.Join(repo, ".git"))
	}
	if projection.CurrentRef != "refs/heads/task-branch" {
		t.Fatalf("CurrentRef = %q", projection.CurrentRef)
	}
	if !containsGitMetadataPath(projection.WritablePaths, projection.GitDir) {
		t.Fatalf("WritablePaths must contain owned GitDir: %#v", projection.WritablePaths)
	}
	if containsGitMetadataPath(projection.WritablePaths, projection.CommonDir) {
		t.Fatalf("WritablePaths must not contain common .git root: %#v", projection.WritablePaths)
	}
	if err := projection.Revalidate(); err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
}

func TestResolveGitMetadataRejectsForgedLinkedWorktreePointer(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)

	gitDir := runGitMetadata(t, checkout, "rev-parse", "--git-dir")
	if err := os.WriteFile(filepath.Join(gitDir, "gitdir"), []byte(filepath.Join(t.TempDir(), ".git")+"\n"), 0o600); err != nil {
		t.Fatalf("forge reciprocal gitdir: %v", err)
	}
	if _, err := ResolveGitMetadata(checkout); err == nil {
		t.Fatal("ResolveGitMetadata accepted forged reciprocal pointer")
	}
}

func TestGitMetadataProjectionPreservesNativeIndexLockAndCommit(t *testing.T) {
	repo := initGitMetadataRepository(t)
	checkout := filepath.Join(t.TempDir(), "task-checkout")
	runGitMetadata(t, repo, "worktree", "add", "-b", "task-branch", checkout)
	projection, err := ResolveGitMetadata(checkout)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(projection.GitDir, "index.lock")
	if err := os.WriteFile(lockPath, []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("git", "-C", checkout, "add", "tracked.txt").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "index.lock") {
		t.Fatalf("git add error=%v output=%s, want native index.lock conflict", err, output)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "tracked.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitMetadata(t, checkout, "add", "tracked.txt")
	runGitMetadata(t, checkout, "commit", "-m", "task change")
	runGitMetadata(t, checkout, "fsck", "--strict")
}

func TestGitMetadataProjectionSupportsMultipleTaskRepositories(t *testing.T) {
	repoA := initGitMetadataRepository(t)
	repoB := initGitMetadataRepository(t)
	checkoutA := filepath.Join(t.TempDir(), "primary")
	checkoutB := filepath.Join(t.TempDir(), "attached")
	runGitMetadata(t, repoA, "worktree", "add", "-b", "task-a", checkoutA)
	runGitMetadata(t, repoB, "worktree", "add", "-b", "task-b", checkoutB)

	projectionA, err := ResolveGitMetadata(checkoutA)
	if err != nil {
		t.Fatal(err)
	}
	projectionB, err := ResolveGitMetadata(checkoutB)
	if err != nil {
		t.Fatal(err)
	}
	if projectionA.GitDir == projectionB.GitDir || projectionA.CommonDir == projectionB.CommonDir {
		t.Fatalf("multi-repository projections overlap: %#v %#v", projectionA, projectionB)
	}
	for _, checkout := range []string{checkoutA, checkoutB} {
		if err := os.WriteFile(filepath.Join(checkout, "tracked.txt"), []byte("changed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGitMetadata(t, checkout, "add", "tracked.txt")
		runGitMetadata(t, checkout, "commit", "-m", "task change")
		runGitMetadata(t, checkout, "fsck", "--strict")
	}
}

func initGitMetadataRepository(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	runGitMetadata(t, "", "init", "-b", "main", repo)
	runGitMetadata(t, repo, "config", "user.email", "test@example.com")
	runGitMetadata(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("initial\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGitMetadata(t, repo, "add", "tracked.txt")
	runGitMetadata(t, repo, "commit", "-m", "initial")
	return repo
}

func runGitMetadata(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmdArgs := args
	if directory != "" {
		cmdArgs = append([]string{"-C", directory}, args...)
	}
	output, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", cmdArgs, err, output)
	}
	return strings.TrimSpace(string(output))
}

func containsGitMetadataPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
