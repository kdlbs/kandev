package gitref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultBranch_RejectsRelativePath — guardRepoPath refuses relative
// paths so callers can't sneak `repos/..` past the I/O boundary even if
// the upstream allowlist check is bypassed.
func TestDefaultBranch_RejectsRelativePath(t *testing.T) {
	if _, err := DefaultBranch("repos/foo"); err == nil {
		t.Fatal("expected error for relative path")
	}
}

// TestDefaultBranch_RejectsTraversal — explicit guard against ".."
// segments. CodeQL's go/path-injection taint analysis recognizes this
// shape as a sanitizer.
func TestDefaultBranch_RejectsTraversal(t *testing.T) {
	if _, err := DefaultBranch("/tmp/../etc"); err == nil {
		t.Fatal("expected error for path containing ..")
	}
}

// TestDefaultBranch_RejectsEmpty — guardRepoPath errors on empty input
// rather than letting filepath.Join silently turn it into ".git".
func TestDefaultBranch_RejectsEmpty(t *testing.T) {
	if _, err := DefaultBranch(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

// TestDefaultBranch_AcceptsAbsolutePathToValidRepo — sanity check that
// the guard doesn't reject legitimate absolute paths. We seed a minimal
// .git directory with a HEAD on main and assert the probe finds it.
func TestDefaultBranch_AcceptsAbsolutePathToValidRepo(t *testing.T) {
	repoPath := t.TempDir()
	if !filepath.IsAbs(repoPath) {
		t.Fatalf("t.TempDir() returned non-absolute path: %q", repoPath)
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	branch, err := DefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.EqualFold(branch, "main") {
		t.Fatalf("expected main, got %q", branch)
	}
}
