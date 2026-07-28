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

// seedOwnedDirectoryLink plants a link through the production creation path so
// the test exercises the same reparse point / symlink a real launch produced.
func seedOwnedDirectoryLink(t *testing.T, root, name, target string) {
	t.Helper()
	if _, err := CreateOwnedDirectoryLink(root, name, target); err != nil {
		t.Skipf("directory link unsupported: %v", err)
	}
}

func TestRemoveSelfReferentialDirectoryLinkRemovesOnlySelfLink(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(keep, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "self", root)

	removed, err := RemoveSelfReferentialDirectoryLink(root, "self")
	if err != nil || !removed {
		t.Fatalf("RemoveSelfReferentialDirectoryLink = %v, %v; want true, nil", removed, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "self")); !os.IsNotExist(err) {
		t.Fatalf("self link still present: %v", err)
	}
	if got, err := os.ReadFile(keep); err != nil || string(got) != "one" {
		t.Fatalf("target content = %q, %v; removal must not touch the target", got, err)
	}
}

func TestRemoveSelfReferentialDirectoryLinkKeepsForeignLink(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "source", target)

	removed, err := RemoveSelfReferentialDirectoryLink(root, "source")
	if err != nil || removed {
		t.Fatalf("RemoveSelfReferentialDirectoryLink = %v, %v; want false, nil", removed, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "source", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("read through kept link = %q, %v", got, err)
	}
}

func TestRemoveSelfReferentialDirectoryLinkKeepsRealDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "api")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := RemoveSelfReferentialDirectoryLink(root, "api")
	if err != nil || removed {
		t.Fatalf("RemoveSelfReferentialDirectoryLink = %v, %v; want false, nil", removed, err)
	}
	if got, err := os.ReadFile(filepath.Join(real, "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("real directory content = %q, %v; a real directory must never be removed", got, err)
	}
}

func TestRemoveSelfReferentialDirectoryLinkIgnoresMissingEntry(t *testing.T) {
	removed, err := RemoveSelfReferentialDirectoryLink(t.TempDir(), "absent")
	if err != nil || removed {
		t.Fatalf("RemoveSelfReferentialDirectoryLink = %v, %v; want false, nil", removed, err)
	}
}

// Every relaunch and resume re-runs Ensure over unchanged links, so a second
// call must be a no-op. A path-text comparison rejected Windows junctions here
// because filepath.EvalSymlinks does not traverse them.
func TestEnsureOwnedDirectoryLinkIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, created, err := EnsureOwnedDirectoryLink(root, "api", target)
	if err != nil || !created {
		t.Skipf("directory link unsupported: created=%v err=%v", created, err)
	}

	second, created, err := EnsureOwnedDirectoryLink(root, "api", target)
	if err != nil {
		t.Fatalf("second EnsureOwnedDirectoryLink: %v; an unchanged link must be reused", err)
	}
	if created {
		t.Fatal("second EnsureOwnedDirectoryLink reported creation")
	}
	if second != first {
		t.Fatalf("second link = %q, want %q", second, first)
	}
	if got, err := os.ReadFile(filepath.Join(second, "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("read through reused link = %q, %v", got, err)
	}
}

func TestEnsureOwnedDirectoryLinkRejectsDifferentTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	target, other := t.TempDir(), t.TempDir()
	if _, _, err := EnsureOwnedDirectoryLink(root, "api", target); err != nil {
		t.Skipf("directory link unsupported: %v", err)
	}

	if _, _, err := EnsureOwnedDirectoryLink(root, "api", other); err == nil {
		t.Fatal("EnsureOwnedDirectoryLink accepted a link pointing elsewhere")
	}
}
