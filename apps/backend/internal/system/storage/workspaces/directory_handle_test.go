package workspaces

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryHandleReadsEntriesFromPinnedDirectory(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "task")
	nested := filepath.Join(root, "cache")
	foreign := filepath.Join(root, "foreign")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "data.txt"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "data.txt"), []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("original-target", filepath.Join(nested, "link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if err := os.Symlink("foreign-target", filepath.Join(foreign, "link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	rootHandle, err := OpenDirectoryNoFollow(parent, root)
	if err != nil {
		t.Fatalf("open task root: %v", err)
	}
	defer func() { _ = rootHandle.Close() }()
	handle, err := rootHandle.OpenSubdirectory("cache")
	if err != nil {
		t.Fatalf("open cache directory: %v", err)
	}
	defer func() { _ = handle.Close() }()

	pinned := nested + ".pinned"
	if err := os.Rename(nested, pinned); err != nil {
		t.Fatalf("rename pinned directory: %v", err)
	}
	if err := os.Symlink(foreign, nested); err != nil {
		t.Fatalf("replace directory with foreign symlink: %v", err)
	}

	target, err := handle.ReadLink("link")
	if err != nil || target != "original-target" {
		t.Fatalf("pinned link target = %q, err = %v; want original target", target, err)
	}
	mode, err := handle.LstatEntry("link")
	if err != nil || mode&os.ModeSymlink == 0 {
		t.Fatalf("pinned link mode = %v, err = %v; want symlink", mode, err)
	}
	entries, err := handle.ReadDir()
	if err != nil {
		t.Fatalf("read pinned directory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("pinned directory entries = %d, want 2", len(entries))
	}
	file, err := handle.OpenFile("data.txt")
	if err != nil {
		t.Fatalf("open pinned file: %v", err)
	}
	content, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(content) != "original" {
		t.Fatalf("pinned file content = %q, read err = %v, close err = %v", content, readErr, closeErr)
	}
}

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
