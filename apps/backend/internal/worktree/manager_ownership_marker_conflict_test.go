package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

// REGRESSION: an inherit_parent child whose parent was archived falls
// through to creating a fresh worktree (its bound environment is gone), and
// finds the shared task directory still marked for the archived owner
// task. Before this fix, the resulting error was the bare
// "workspace ownership marker conflicts with requested task root" with no
// indication of which task owns the directory or what to do about it.
func TestPrepareTaskWorktreePath_OwnershipConflictNamesConflictingOwner(t *testing.T) {
	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	taskDirName := "shared-task-dir"
	taskDir := filepath.Join(cfg.TasksBasePath, taskDirName)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(taskDir, storageworkspaces.OwnershipMarker{
		TaskID:        "task-parent-archived",
		TaskDirName:   taskDirName,
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write parent ownership marker: %v", err)
	}

	_, err = mgr.prepareTaskWorktreePath(CreateRequest{
		TaskID:      "task-child-orphaned",
		TaskDirName: taskDirName,
		RepoName:    "my-repo",
	})
	if err == nil {
		t.Fatal("expected an ownership marker conflict error")
	}
	if !strings.Contains(err.Error(), "task-parent-archived") {
		t.Fatalf("error %q does not name the conflicting owner task", err.Error())
	}
	if !strings.Contains(err.Error(), "workspace_mode=new_workspace") {
		t.Fatalf("error %q does not mention the workaround", err.Error())
	}
}

// A directory with no pre-existing marker (the ordinary first-write case)
// must still get a plain, unenriched wrapped error and must not spuriously
// claim a conflicting owner when WriteOwnershipMarker fails for some other
// reason.
func TestPrepareTaskWorktreePath_NoConflictWhenNoExistingMarker(t *testing.T) {
	cfg := newTestConfig(t)
	store := newMockStore()
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = mgr.prepareTaskWorktreePath(CreateRequest{
		TaskID:      "task-fresh",
		TaskDirName: "fresh-task-dir",
		RepoName:    "my-repo",
	})
	if err != nil {
		t.Fatalf("prepareTaskWorktreePath() unexpected error on first write: %v", err)
	}
}
