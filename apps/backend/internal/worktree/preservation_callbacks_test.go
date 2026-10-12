package worktree

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.3
func TestPreservationDoesNotExecuteMutatingFsmonitor(t *testing.T) {
	repositoryPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	runGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/pr-branch")
	target := filepath.Join(worktreePath, "README.md")
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(t.TempDir(), "fsmonitor")
	script := "#!/bin/sh\nprintf 'mutated by fsmonitor\\n' > '" + filepath.ToSlash(target) + "'\nprintf '\\000'\n"
	if err := os.WriteFile(hook, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runGit(t, repositoryPath, "config", "core.fsmonitor", filepath.ToSlash(hook))
	indexPath := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "--git-path", "index"))
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	evidence, inspectErr := InspectPreservedCheckout(context.Background(), PreservationRequest{RepositoryPath: repositoryPath, WorktreePath: worktreePath, ExpectedBranch: "feature/pr-branch", WorktreeID: "review-worktree"})
	if inspectErr != nil {
		t.Fatalf("safe inspection failed: %v", inspectErr)
	}
	indexAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Fatal("inspection changed index bytes")
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("preservation inspector mutated checkout through fsmonitor: before=%q after=%q inspection_error=%v evidence=%+v", before, after, inspectErr, evidence)
	}
	runGit(t, worktreePath, "status", "--porcelain")
	control, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, control) {
		t.Fatal("positive control did not execute fsmonitor")
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.3
func TestPreservationRejectsExternalCleanFilter(t *testing.T) {
	repositoryPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "preserved")
	runGit(t, repositoryPath, "worktree", "add", worktreePath, "feature/pr-branch")
	target := filepath.Join(worktreePath, "README.md")
	marker := filepath.Join(t.TempDir(), "filter-ran")
	script := filepath.Join(t.TempDir(), "clean-filter")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf invoked > '"+filepath.ToSlash(marker)+"'\ncat\n"), 0700); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "config", "filter.preservation.clean", filepath.ToSlash(script))
	if err := os.WriteFile(filepath.Join(worktreePath, ".gitattributes"), []byte("README.md filter=preservation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "status", "--porcelain")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("positive control did not execute filter: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	_, err := InspectPreservedCheckout(context.Background(), PreservationRequest{RepositoryPath: repositoryPath, WorktreePath: worktreePath, ExpectedBranch: "feature/pr-branch", WorktreeID: "filter-worktree"})
	if !errors.Is(err, ErrPreservedCheckoutUnproven) {
		t.Fatalf("expected unsafe filter rejection, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("inspection executed filter: %v", err)
	}
}
