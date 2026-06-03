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
	expandedBase, err := cfg.ExpandedTasksBasePath()
	if err != nil {
		t.Fatalf("ExpandedTasksBasePath: %v", err)
	}
	if _, err := os.Stat(expandedBase); err != nil {
		t.Errorf("TasksBasePath %s must survive cleanup: %v", expandedBase, err)
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

// TestCleanupWorktrees_MultiBranchSiblingPreservesParent covers the
// multi-branch task layout: two worktrees for the same task live as
// siblings under {taskDir}. Removing one must keep {taskDir} alive
// (the other sibling is still there). Removing both must clear it.
func TestCleanupWorktrees_MultiBranchSiblingPreservesParent(t *testing.T) {
	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

	primary, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-multi",
		SessionID:      "session-multi",
		TaskTitle:      "Multi Branch",
		RepositoryID:   "repo-multi",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-multi_aaa",
		RepoName:       "repo-multi",
	})
	if err != nil {
		t.Fatalf("Create primary: %v", err)
	}

	sibling, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-multi",
		SessionID:      "session-multi",
		TaskTitle:      "Multi Branch",
		RepositoryID:   "repo-multi",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/pr-branch",
		TaskDirName:    "task-multi_aaa",
		RepoName:       "repo-multi",
		BranchSlug:     "pr",
	})
	if err != nil {
		t.Fatalf("Create sibling: %v", err)
	}

	taskDir := filepath.Dir(primary.Path)
	if filepath.Dir(sibling.Path) != taskDir {
		t.Fatalf("siblings must share parent: primary=%s sibling=%s", primary.Path, sibling.Path)
	}

	// Remove primary first — sibling still occupies taskDir, parent must survive.
	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{primary}); err != nil {
		t.Fatalf("CleanupWorktrees primary: %v", err)
	}
	if _, err := os.Stat(primary.Path); !os.IsNotExist(err) {
		t.Errorf("primary worktree %s should be gone; stat err=%v", primary.Path, err)
	}
	if _, err := os.Stat(taskDir); err != nil {
		t.Errorf("task dir %s must survive while sibling is present: %v", taskDir, err)
	}
	if _, err := os.Stat(sibling.Path); err != nil {
		t.Errorf("sibling worktree %s must survive: %v", sibling.Path, err)
	}

	// Remove sibling — parent should now be cleared.
	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{sibling}); err != nil {
		t.Fatalf("CleanupWorktrees sibling: %v", err)
	}
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Errorf("task dir %s should be removed after last sibling cleared; stat err=%v", taskDir, err)
	}
}

// TestCleanupWorktrees_RemovesEmptyTaskDir_TrailingSlashTasksBase guards
// the path-normalization guard in tryRemoveEmptyTaskDir: a configured
// TasksBasePath with a trailing separator must still match the cleaned
// parent path computed via filepath.Dir.
func TestCleanupWorktrees_RemovesEmptyTaskDir_TrailingSlashTasksBase(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.TasksBasePath += string(filepath.Separator)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoPath := initGitRepoWithRemote(t)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "task-slash",
		SessionID:      "session-slash",
		TaskTitle:      "Trailing Slash",
		RepositoryID:   "repo-slash",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		TaskDirName:    "task-slash_aaa",
		RepoName:       "repo-slash",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	taskDir := filepath.Dir(wt.Path)
	if err := mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}); err != nil {
		t.Fatalf("CleanupWorktrees: %v", err)
	}
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Errorf("task dir %s should be removed despite trailing slash in TasksBasePath; stat err=%v", taskDir, err)
	}
}
