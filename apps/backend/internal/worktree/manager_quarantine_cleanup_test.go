package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestPruneQuarantinedWorkspacePreservesOtherRecoverableRegistration(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	store := newMockStore()
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	originalRoot := t.TempDir()
	quarantineRoot := t.TempDir()
	firstPath := filepath.Join(originalRoot, "task-1", "repo")
	secondPath := filepath.Join(originalRoot, "task-2", "repo")
	runGit(t, repoPath, "branch", "task-1")
	runGit(t, repoPath, "branch", "task-2")
	runGit(t, repoPath, "worktree", "add", firstPath, "task-1")
	runGit(t, repoPath, "worktree", "add", secondPath, "task-2")

	firstQuarantine := filepath.Join(quarantineRoot, "entry-1")
	secondQuarantine := filepath.Join(quarantineRoot, "entry-2")
	if err := os.Rename(filepath.Dir(firstPath), firstQuarantine); err != nil {
		t.Fatalf("quarantine first workspace: %v", err)
	}
	if err := os.Rename(filepath.Dir(secondPath), secondQuarantine); err != nil {
		t.Fatalf("quarantine second workspace: %v", err)
	}
	store.worktrees["wt-1"] = &Worktree{
		ID: "wt-1", TaskID: "task-1", RepositoryPath: repoPath, Path: firstPath,
	}
	store.worktrees["wt-2"] = &Worktree{
		ID: "wt-2", TaskID: "task-2", RepositoryPath: repoPath, Path: secondPath,
	}

	if err := mgr.PruneQuarantinedWorkspace(context.Background(), storage.QuarantineEntry{TaskID: "task-1"}); err != nil {
		t.Fatalf("PruneQuarantinedWorkspace: %v", err)
	}
	worktreeList := runGit(t, repoPath, "worktree", "list", "--porcelain")
	if strings.Contains(worktreeList, "worktree "+firstPath) {
		t.Fatalf("selected worktree registration was preserved:\n%s", worktreeList)
	}
	if err := mgr.PruneQuarantinedWorkspace(context.Background(), storage.QuarantineEntry{TaskID: "task-1"}); err != nil {
		t.Fatalf("repeat PruneQuarantinedWorkspace: %v", err)
	}
	if err := os.Rename(secondQuarantine, filepath.Dir(secondPath)); err != nil {
		t.Fatalf("restore second workspace: %v", err)
	}
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = secondPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("restored sibling worktree is unusable: %v\n%s", err, output)
	}
}

func TestPruneQuarantinedWorkspaceWaitsForRepositoryLock(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	store := newMockStore()
	mgr, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	worktreePath := filepath.Join(t.TempDir(), "task-1", "repo")
	runGit(t, repoPath, "branch", "task-1")
	runGit(t, repoPath, "worktree", "add", worktreePath, "task-1")
	store.worktrees["wt-1"] = &Worktree{
		ID: "wt-1", TaskID: "task-1", RepositoryPath: repoPath, Path: worktreePath,
	}

	repoLock := mgr.getRepoLock(repoPath)
	repoLock.Lock()
	lockHeld := true
	t.Cleanup(func() {
		if lockHeld {
			repoLock.Unlock()
			mgr.releaseRepoLock(repoPath)
		}
	})

	done := make(chan error, 1)
	go func() {
		done <- mgr.PruneQuarantinedWorkspace(
			context.Background(),
			storage.QuarantineEntry{TaskID: "task-1"},
		)
	}()

	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		mgr.repoLockMu.Lock()
		entry := mgr.repoLocks[repoPath]
		waitingForLock := entry != nil && entry.refCount == 2
		mgr.repoLockMu.Unlock()
		if waitingForLock {
			break
		}
		select {
		case err := <-done:
			repoLock.Unlock()
			mgr.releaseRepoLock(repoPath)
			lockHeld = false
			t.Fatalf("PruneQuarantinedWorkspace returned while repository lock was held: %v", err)
		case <-deadline.C:
			t.Fatal("PruneQuarantinedWorkspace did not acquire the repository lock")
		case <-ticker.C:
		}
	}
	select {
	case err := <-done:
		t.Fatalf("PruneQuarantinedWorkspace returned while waiting for repository lock: %v", err)
	default:
	}

	repoLock.Unlock()
	mgr.releaseRepoLock(repoPath)
	lockHeld = false
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("PruneQuarantinedWorkspace: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PruneQuarantinedWorkspace did not resume after repository lock was released")
	}
}

func TestCreateInTaskDirRejectsSymlinkedTaskDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	repoPath := initGitRepoForWorktreeTest(t)
	tasksBase := t.TempDir()
	outside := t.TempDir()
	linkedParent := filepath.Join(tasksBase, "linked-parent")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatalf("create task-directory symlink: %v", err)
	}
	cfg := newTestConfig(t)
	cfg.TasksBasePath = tasksBase
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_, err = mgr.createInTaskDir(context.Background(), CreateRequest{
		TaskID: "task-1", SessionID: "session-1", RepositoryID: "repo-1",
		RepositoryPath: repoPath, BaseBranch: "main", TaskDirName: "linked-parent/task-1", RepoName: "repo",
	}, "main", "", "")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("createInTaskDir() error = %v, want symlink rejection", err)
	}
}
