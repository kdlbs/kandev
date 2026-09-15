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
