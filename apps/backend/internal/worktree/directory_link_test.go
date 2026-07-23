package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateOwnedDirectoryLinkCreatesLiveLinkInsideOwnedRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	target := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	link, err := CreateOwnedDirectoryLink(root, "source", target)
	if err != nil {
		t.Fatalf("CreateOwnedDirectoryLink: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(link, "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("read through link = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(link, "live.txt")); err != nil || string(got) != "two" {
		t.Fatalf("link is not live: %q, %v", got, err)
	}
}

func TestCreateOwnedDirectoryLinkRejectsCollision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	target := t.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOwnedDirectoryLink(root, "source", target); err == nil {
		t.Fatal("CreateOwnedDirectoryLink succeeded for collision")
	}
}

func TestCreateOwnedDirectoryLinkRejectsSymlinkedControlAncestor(t *testing.T) {
	realBase := t.TempDir()
	linkBase := filepath.Join(t.TempDir(), "tasks")
	if err := os.Symlink(realBase, linkBase); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := CreateOwnedDirectoryLink(filepath.Join(linkBase, "task-1"), "source", t.TempDir()); err == nil {
		t.Fatal("CreateOwnedDirectoryLink accepted symlinked control ancestor")
	}
}
