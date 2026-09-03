package workspaces

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryHandlePinsWorktreeAcrossPathReplacement(t *testing.T) {
	parent := t.TempDir()
	original := filepath.Join(parent, "original")
	replacement := filepath.Join(parent, "replacement")
	if err := os.MkdirAll(original, 0o755); err != nil {
		t.Fatalf("mkdir original: %v", err)
	}
	if err := os.MkdirAll(replacement, 0o755); err != nil {
		t.Fatalf("mkdir replacement: %v", err)
	}
	if err := os.WriteFile(filepath.Join(original, ".git"), []byte("gitdir: original\n"), 0o600); err != nil {
		t.Fatalf("write original git file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(replacement, ".git"), []byte("gitdir: replacement\n"), 0o600); err != nil {
		t.Fatalf("write replacement git file: %v", err)
	}

	handle, err := OpenDirectoryNoFollow(parent, original)
	if err != nil {
		t.Fatalf("open original directory: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	archived := original + ".archived"
	if err := os.Rename(original, archived); err != nil {
		t.Fatalf("rename original directory: %v", err)
	}
	if err := os.Rename(replacement, original); err != nil {
		t.Fatalf("replace original directory: %v", err)
	}

	if !handle.IsValidWorktree() {
		t.Fatal("opened directory no longer validates as the original worktree")
	}
	if err := handle.VerifyPath(original); err == nil {
		t.Fatal("VerifyPath succeeded after the lexical path changed")
	}
	if err := handle.RemoveDirectory(context.Background()); err == nil {
		t.Fatal("remove pinned original directory succeeded after path replacement")
	}
	if _, err := os.Stat(filepath.Join(archived, ".git")); !os.IsNotExist(err) {
		t.Fatalf("archived original directory contents remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(original, ".git")); err != nil {
		t.Fatalf("replacement directory changed: %v", err)
	}
}

func TestCreateDirectoryNoFollowPinsCreatedTaskRootBeforeMarkerWrite(t *testing.T) {
	parent := t.TempDir()
	tasksBase := filepath.Join(parent, "tasks")
	taskRoot := filepath.Join(tasksBase, "task-one")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	handle, err := CreateDirectoryNoFollow(tasksBase, taskRoot, 0o755)
	if err != nil {
		t.Fatalf("create task root: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	archived := tasksBase + ".archived"
	if err := os.Rename(tasksBase, archived); err != nil {
		t.Fatalf("rename tasks base: %v", err)
	}
	if err := os.Symlink(outside, tasksBase); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	if err := handle.VerifyPath(taskRoot); err == nil {
		t.Fatal("VerifyPath succeeded after the task root was replaced")
	}
	if err := WriteOwnershipMarkerNoFollow(handle, OwnershipMarker{
		TaskID:        "task-one",
		TaskDirName:   "task-one",
		LayoutVersion: LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write marker through opened task root: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, OwnershipMarkerFilename)); !os.IsNotExist(err) {
		t.Fatalf("marker escaped into replacement target: %v", err)
	}
}
