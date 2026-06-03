package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCleanupWorktrees_RemovesEmptyTaskDir is a regression guard for issue
// #1266. The worktree manager must remove BOTH the inner {repoName}/ subdir
// AND the now-empty parent {taskDirName}/ container that nests it. Before
// the fix, only the inner subdir was removed; archived tasks accumulated
// empty parent husks under ~/.kandev/tasks/.
func TestCleanupWorktrees_RemovesEmptyTaskDir(t *testing.T) {
	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		TaskTitle:      "Empty Dir Repro",
		RepositoryID:   "repo-1",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-1_xyz",
		RepoName:       "repo-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	taskDir := filepath.Dir(wt.Path)
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatalf("worktree path %s should exist before cleanup: %v", wt.Path, err)
	}
	if _, err := os.Stat(taskDir); err != nil {
		t.Fatalf("task dir %s should exist before cleanup: %v", taskDir, err)
	}

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree path %s should be removed; stat err=%v", wt.Path, err)
	}
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Errorf("parent task dir %s should be removed; stat err=%v", taskDir, err)
	}

	// TasksBasePath itself must NOT be removed.
	if _, err := os.Stat(cfg.TasksBasePath); err != nil {
		t.Errorf("TasksBasePath %s must survive cleanup: %v", cfg.TasksBasePath, err)
	}
}

// TestCleanupWorktrees_PreservesNonEmptyTaskDir verifies that workspace-
// scoped content (or a sibling worktree from another session) left under
// the task directory is preserved when one worktree is removed.
func TestCleanupWorktrees_PreservesNonEmptyTaskDir(t *testing.T) {
	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-keep",
		SessionID:      "session-keep",
		TaskTitle:      "Keep Parent",
		RepositoryID:   "repo-keep",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-keep_aaa",
		RepoName:       "repo-keep",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	taskDir := filepath.Dir(wt.Path)
	// Drop a sibling file representing workspace-scoped content.
	leftover := filepath.Join(taskDir, "leftover.txt")
	if err := os.WriteFile(leftover, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write leftover: %v", err)
	}

	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree subdir %s should be removed; stat err=%v", wt.Path, err)
	}
	if _, err := os.Stat(taskDir); err != nil {
		t.Errorf("non-empty task dir %s must be preserved: %v", taskDir, err)
	}
	if _, err := os.Stat(leftover); err != nil {
		t.Errorf("leftover file %s must survive: %v", leftover, err)
	}
}
