//go:build !windows

package workspaces

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDirectoryNoFollowRejectsSymlinkedManagedRoot(t *testing.T) {
	parent := t.TempDir()
	tasksBase := filepath.Join(parent, "tasks")
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, tasksBase); err != nil {
		t.Fatalf("symlink tasks base: %v", err)
	}

	_, err := CreateDirectoryNoFollow(tasksBase, filepath.Join(tasksBase, "task-one"), 0o755)
	if err == nil {
		t.Fatal("CreateDirectoryNoFollow followed a symlinked tasks base")
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "task-one")); !os.IsNotExist(statErr) {
		t.Fatalf("symlink target was modified: %v", statErr)
	}
}
