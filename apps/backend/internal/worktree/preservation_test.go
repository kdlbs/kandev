package worktree

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3
func TestInspectPreservedCheckoutHashesDirtyAndUntrackedState(t *testing.T) {
	repositoryPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	runGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/pr-branch")
	if err := os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "untracked.txt"), []byte("unique\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	evidence, err := InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath,
		WorktreePath:   worktreePath,
		ExpectedBranch: "feature/pr-branch",
		WorktreeID:     "synthetic-worktree",
	})
	if err != nil {
		t.Fatalf("InspectPreservedCheckout: %v", err)
	}
	if evidence.ObservedBranch != "feature/pr-branch" || evidence.HeadOID == "" || evidence.RefName != "refs/heads/feature/pr-branch" {
		t.Fatalf("identity evidence = %+v", evidence)
	}
	if evidence.DirtyCount != 1 || evidence.UntrackedCount != 1 || evidence.StatusHash == "" || evidence.ContentHash == "" || evidence.PathHash == "" {
		t.Fatalf("preservation evidence = %+v", evidence)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3
func TestInspectPreservedCheckoutHashesIgnoredState(t *testing.T) {
	repositoryPath := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(repositoryPath, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repositoryPath, "add", ".gitignore")
	runGit(t, repositoryPath, "commit", "-m", "ignore local file")
	runGit(t, repositoryPath, "branch", "-f", "feature/pr-branch", "main")
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	runGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/pr-branch")
	if err := os.WriteFile(filepath.Join(worktreePath, "ignored.txt"), []byte("unique-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath,
		WorktreePath:   worktreePath,
		ExpectedBranch: "feature/pr-branch",
		WorktreeID:     "synthetic-worktree",
	})
	if err != nil {
		t.Fatalf("InspectPreservedCheckout(before): %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "ignored.txt"), []byte("unique-b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath,
		WorktreePath:   worktreePath,
		ExpectedBranch: "feature/pr-branch",
		WorktreeID:     "synthetic-worktree",
	})
	if err != nil {
		t.Fatalf("InspectPreservedCheckout(after): %v", err)
	}
	if before.ContentHash == after.ContentHash {
		t.Fatalf("ignored file content did not affect preservation hash: before=%+v after=%+v", before, after)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.6
// TestInspectPreservedCheckoutDoesNotMutateIndex proves the read-only
// inspection required by the preservation contract: a status-only probe
// must never write to .git/index, even incidentally via Git's normal
// opportunistic stat-cache refresh.
func TestInspectPreservedCheckoutDoesNotMutateIndex(t *testing.T) {
	repositoryPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	runGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/pr-branch")
	// A stale on-disk mtime relative to the index's cached stat info is what
	// triggers git status's opportunistic index refresh; touching the file
	// after checkout reproduces that condition deterministically.
	readmePath := filepath.Join(worktreePath, "README.md")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(readmePath, future, future); err != nil {
		t.Fatal(err)
	}
	indexFile := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "--path-format=absolute", "--git-path", "index"))
	before, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath,
		WorktreePath:   worktreePath,
		ExpectedBranch: "feature/pr-branch",
		WorktreeID:     "synthetic-worktree",
	}); err != nil {
		t.Fatalf("InspectPreservedCheckout: %v", err)
	}

	after, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("InspectPreservedCheckout mutated .git index: before=%d bytes after=%d bytes", len(before), len(after))
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.6
// TestInspectPreservedCheckoutDetectsStagedIndexOnlyDivergence proves the
// blind spot ContentHash alone has: ContentHash only ever reads
// working-tree bytes (via `git ls-files -co` plus os.ReadFile), so a change
// that touches only the staged index — for example a low-level
// `update-index --cacheinfo` write that repoints a path's staged blob SHA
// without touching the working-tree file — leaves ContentHash unchanged.
// IndexHash reads the index directly and must catch it.
func TestInspectPreservedCheckoutDetectsStagedIndexOnlyDivergence(t *testing.T) {
	repositoryPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	runGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/pr-branch")

	before, err := InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath,
		WorktreePath:   worktreePath,
		ExpectedBranch: "feature/pr-branch",
		WorktreeID:     "synthetic-worktree",
	})
	if err != nil {
		t.Fatalf("InspectPreservedCheckout(before): %v", err)
	}

	// git hash-object -w writes a new blob to the object database (allowed:
	// it never touches an existing object, ref, or the working tree) so the
	// index can be repointed at different staged content without altering
	// README.md on disk at all.
	blobSHA := strings.TrimSpace(runGitWithStdin(t, worktreePath, "diverged staged content\n", "hash-object", "-w", "--stdin"))
	runGit(t, worktreePath, "update-index", "--cacheinfo", "100644,"+blobSHA+",README.md")

	after, err := InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath,
		WorktreePath:   worktreePath,
		ExpectedBranch: "feature/pr-branch",
		WorktreeID:     "synthetic-worktree",
	})
	if err != nil {
		t.Fatalf("InspectPreservedCheckout(after): %v", err)
	}

	if before.ContentHash != after.ContentHash {
		t.Fatalf("working-tree bytes were untouched, so ContentHash must not diverge: before=%s after=%s", before.ContentHash, after.ContentHash)
	}
	if before.IndexHash == after.IndexHash {
		t.Fatalf("staged-index-only change must be detected via IndexHash: before=%s after=%s", before.IndexHash, after.IndexHash)
	}
}

func runGitWithStdin(t *testing.T, repoPath, stdin string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	cmd.Stdin = strings.NewReader(stdin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
func TestInspectPreservedCheckoutRejectsWrongBranchAndSymlink(t *testing.T) {
	repositoryPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	runGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/pr-branch")

	_, err := InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath, WorktreePath: worktreePath,
		ExpectedBranch: "feature/different", WorktreeID: "synthetic-worktree",
	})
	if !errors.Is(err, ErrPreservedCheckoutUnproven) {
		t.Fatalf("wrong branch error = %v", err)
	}

	symlinkPath := filepath.Join(t.TempDir(), "preserved-link")
	if err := os.Symlink(worktreePath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	_, err = InspectPreservedCheckout(context.Background(), PreservationRequest{
		RepositoryPath: repositoryPath, WorktreePath: symlinkPath,
		ExpectedBranch: "feature/pr-branch", WorktreeID: "synthetic-worktree",
	})
	if !errors.Is(err, ErrPreservedCheckoutUnproven) {
		t.Fatalf("symlink error = %v", err)
	}
}
