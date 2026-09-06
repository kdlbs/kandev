//go:build !windows

package workspaces

import (
	"os"
	"path/filepath"
	"testing"
)

// This exercises the pinning guarantee through a symlink replacement of the
// managed base. It is Unix-only: it renames a directory tree while a child
// handle is pinned, which Windows refuses regardless of share flags, so the
// scenario cannot be reproduced there. The Windows pinning/identity guarantees
// are covered in directory_handle_windows_test.go and by
// TestDirectoryHandlePinsWorktreeAcrossPathReplacement.
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
