package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
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
