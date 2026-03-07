package worktree

import (
	"strings"
	"testing"
)

func TestBuildWorktreeNames(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()
	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	t.Run("semantic naming with task title", func(t *testing.T) {
		req := CreateRequest{
			TaskID:               "task-abc",
			TaskTitle:            "Fix login bug",
			WorktreeBranchPrefix: "kandev/",
		}
		dirName, branchName := mgr.buildWorktreeNames(req)

		// Dir should contain sanitized title and a UUID suffix
		if !strings.HasPrefix(dirName, "fix-login-bug_") {
			t.Errorf("dirName = %q, want prefix %q", dirName, "fix-login-bug_")
		}
		// UUID suffix should be 8 chars
		parts := strings.SplitN(dirName, "_", 2)
		if len(parts) != 2 || len(parts[1]) != 8 {
			t.Errorf("dirName suffix should be 8 chars, got %q", dirName)
		}

		// Branch should have prefix + sanitized title + suffix
		if !strings.HasPrefix(branchName, "kandev/fix-login-bug-") {
			t.Errorf("branchName = %q, want prefix %q", branchName, "kandev/fix-login-bug-")
		}
	})

	t.Run("fallback to task ID when title is empty", func(t *testing.T) {
		req := CreateRequest{
			TaskID:               "task-xyz-123",
			TaskTitle:            "",
			WorktreeBranchPrefix: "kandev/",
		}
		dirName, branchName := mgr.buildWorktreeNames(req)

		if !strings.HasPrefix(dirName, "task-xyz-123_") {
			t.Errorf("dirName = %q, want prefix %q", dirName, "task-xyz-123_")
		}
		if !strings.HasPrefix(branchName, "kandev/task-xyz-123-") {
			t.Errorf("branchName = %q, want prefix %q", branchName, "kandev/task-xyz-123-")
		}
	})

	t.Run("empty branch prefix", func(t *testing.T) {
		req := CreateRequest{
			TaskID:               "task-1",
			TaskTitle:            "Add feature",
			WorktreeBranchPrefix: "",
		}
		_, branchName := mgr.buildWorktreeNames(req)

		// Default prefix from NormalizeBranchPrefix("")
		defaultPrefix := NormalizeBranchPrefix("")
		if !strings.HasPrefix(branchName, defaultPrefix+"add-feature-") {
			t.Errorf("branchName = %q, want prefix %q", branchName, defaultPrefix+"add-feature-")
		}
	})

	t.Run("title with only special chars falls back to task ID for branch", func(t *testing.T) {
		req := CreateRequest{
			TaskID:               "task-fallback",
			TaskTitle:            "!@#$%",
			WorktreeBranchPrefix: "kandev/",
		}
		_, branchName := mgr.buildWorktreeNames(req)

		// SanitizeForBranch("!@#$%") returns "", so should fall back to task ID
		if !strings.HasPrefix(branchName, "kandev/task-fallback-") {
			t.Errorf("branchName = %q, want prefix %q", branchName, "kandev/task-fallback-")
		}
	})

	t.Run("unique names on successive calls", func(t *testing.T) {
		req := CreateRequest{
			TaskID:    "task-1",
			TaskTitle: "Same title",
		}
		dir1, branch1 := mgr.buildWorktreeNames(req)
		dir2, branch2 := mgr.buildWorktreeNames(req)

		if dir1 == dir2 {
			t.Errorf("expected unique dir names, both are %q", dir1)
		}
		if branch1 == branch2 {
			t.Errorf("expected unique branch names, both are %q", branch1)
		}
	})
}
