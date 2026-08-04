package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

// canonicalTempDir resolves symlinks in t.TempDir() so tests hand production
// code a canonical owned control root (macOS /var -> /private/var).
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks(TempDir): %v", err)
	}
	return d
}

func TestCreateOwnedDirectoryLinkCreatesLiveLinkInsideOwnedRoot(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
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
// Creation failure is fatal, not a skip: a directory junction needs no elevation
// on Windows and a symlink is always available on the Unix runners, so a failure
// here is a real regression rather than an unsupported platform.
func seedOwnedDirectoryLink(t *testing.T, root, name, target string) {
	t.Helper()
	if _, err := CreateOwnedDirectoryLink(root, name, target); err != nil {
		t.Fatalf("CreateOwnedDirectoryLink: %v", err)
	}
}

// The predicate only reports. Every case below additionally asserts that the
// inspected entry is still on disk afterwards, since it may be a link the user
// or the repository keeps on purpose.
func TestIsSelfReferentialDirectoryLinkDetectsSelfLink(t *testing.T) {
	root := canonicalTempDir(t)
	seedOwnedDirectoryLink(t, root, "self", root)

	selfLink, err := IsSelfReferentialDirectoryLink(root, "self")
	if err != nil || !selfLink {
		t.Fatalf("IsSelfReferentialDirectoryLink = %v, %v; want true, nil", selfLink, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "self")); err != nil {
		t.Fatalf("entry was removed: %v", err)
	}
}

func TestIsSelfReferentialDirectoryLinkIgnoresForeignLink(t *testing.T) {
	root, target := canonicalTempDir(t), t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "source", target)

	selfLink, err := IsSelfReferentialDirectoryLink(root, "source")
	if err != nil || selfLink {
		t.Fatalf("IsSelfReferentialDirectoryLink = %v, %v; want false, nil", selfLink, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "source", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("read through kept link = %q, %v", got, err)
	}
}

func TestIsSelfReferentialDirectoryLinkIgnoresRealDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "api")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	selfLink, err := IsSelfReferentialDirectoryLink(root, "api")
	if err != nil || selfLink {
		t.Fatalf("IsSelfReferentialDirectoryLink = %v, %v; want false, nil", selfLink, err)
	}
	if got, err := os.ReadFile(filepath.Join(real, "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("real directory content = %q, %v", got, err)
	}
}

func TestIsSelfReferentialDirectoryLinkIgnoresMissingEntry(t *testing.T) {
	selfLink, err := IsSelfReferentialDirectoryLink(t.TempDir(), "absent")
	if err != nil || selfLink {
		t.Fatalf("IsSelfReferentialDirectoryLink = %v, %v; want false, nil", selfLink, err)
	}
}

// Every relaunch and resume re-runs Ensure over unchanged links, so a second
// call must be a no-op. A path-text comparison rejected Windows junctions here
// because filepath.EvalSymlinks does not traverse them.
func TestEnsureOwnedDirectoryLinkIsIdempotent(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, created, err := EnsureOwnedDirectoryLink(root, "api", target)
	if err != nil || !created {
		t.Fatalf("first EnsureOwnedDirectoryLink: created=%v err=%v", created, err)
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
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	target, other := t.TempDir(), t.TempDir()
	if _, _, err := EnsureOwnedDirectoryLink(root, "api", target); err != nil {
		t.Fatalf("EnsureOwnedDirectoryLink: %v", err)
	}

	if _, _, err := EnsureOwnedDirectoryLink(root, "api", other); err == nil {
		t.Fatal("EnsureOwnedDirectoryLink accepted a link pointing elsewhere")
	}
}
