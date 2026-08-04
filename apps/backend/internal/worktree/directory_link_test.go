package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
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

func testOwnedDirectoryLinkOwner() OwnedDirectoryLinkOwner {
	return OwnedDirectoryLinkOwner{TaskID: "task-1", TaskDirName: "task-1"}
}

func writeOwnershipMarker(t *testing.T, root string, owner OwnedDirectoryLinkOwner) {
	t.Helper()
	if err := storageworkspaces.WriteOwnershipMarker(root, storageworkspaces.OwnershipMarker{
		TaskID: owner.TaskID, TaskDirName: owner.TaskDirName, LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("WriteOwnershipMarker: %v", err)
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
	first, err := EnsureOwnedDirectoryLink(root, "api", target, testOwnedDirectoryLinkOwner())
	if err != nil || !first.Created {
		t.Fatalf("first EnsureOwnedDirectoryLink: created=%v err=%v", first.Created, err)
	}

	second, err := EnsureOwnedDirectoryLink(root, "api", target, testOwnedDirectoryLinkOwner())
	if err != nil {
		t.Fatalf("second EnsureOwnedDirectoryLink: %v; an unchanged link must be reused", err)
	}
	if second.Created {
		t.Fatal("second EnsureOwnedDirectoryLink reported creation")
	}
	if second.Path != first.Path {
		t.Fatalf("second link = %q, want %q", second.Path, first.Path)
	}
	if got, err := os.ReadFile(filepath.Join(second.Path, "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("read through reused link = %q, %v", got, err)
	}
}

// A Kandev-owned task root is Kandev's to reconcile: an owned directory link is
// a pointer, not content, so a stale target left by an earlier launch must be
// repointed rather than wedging every launch and resume with a target mismatch.
func TestEnsureOwnedDirectoryLinkRepointsOwnedLinkOnMismatch(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	target, other := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "live.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "api", target)

	result, err := EnsureOwnedDirectoryLink(root, "api", other, testOwnedDirectoryLinkOwner())
	if err != nil {
		t.Fatalf("EnsureOwnedDirectoryLink repoint: %v", err)
	}
	if !result.Created {
		t.Fatal("EnsureOwnedDirectoryLink did not report a recreate on mismatch")
	}
	linkInfo, err := os.Stat(result.Path)
	if err != nil {
		t.Fatalf("stat repointed link: %v", err)
	}
	otherInfo, err := os.Stat(other)
	if err != nil {
		t.Fatalf("stat new target: %v", err)
	}
	if !os.SameFile(linkInfo, otherInfo) {
		t.Fatal("link was not repointed to the new target")
	}
	if got, err := os.ReadFile(filepath.Join(result.Path, "live.txt")); err != nil || string(got) != "two" {
		t.Fatalf("read through repointed link = %q, %v", got, err)
	}
}

func TestEnsureOwnedDirectoryLinkRepointsOwnedLinkWithMatchingMarker(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	current, other := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "live.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "api", current)
	writeOwnershipMarker(t, root, testOwnedDirectoryLinkOwner())

	result, err := EnsureOwnedDirectoryLink(root, "api", other, testOwnedDirectoryLinkOwner())
	if err != nil {
		t.Fatalf("EnsureOwnedDirectoryLink with matching marker: %v", err)
	}
	if !result.Created || result.PriorTarget == "" {
		t.Fatalf("result = %+v, want Created with a prior target", result)
	}
	if got, err := os.ReadFile(filepath.Join(result.Path, "live.txt")); err != nil || string(got) != "two" {
		t.Fatalf("read through matching-marker repoint = %q, %v", got, err)
	}
}

func TestEnsureOwnedDirectoryLinkRejectsMarkerConflictOnMismatch(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	current, other := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(current, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "live.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "api", current)
	writeOwnershipMarker(t, root, testOwnedDirectoryLinkOwner())

	_, err := EnsureOwnedDirectoryLink(root, "api", other, OwnedDirectoryLinkOwner{TaskID: "task-2", TaskDirName: "task-2"})
	if err == nil || !strings.Contains(err.Error(), errWorkspaceOwnershipMarkerConflict) {
		t.Fatalf("EnsureOwnedDirectoryLink error = %v, want marker conflict", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("marker-conflict repoint disturbed original link = %q, %v", got, err)
	}
}

func TestEnsureOwnedDirectoryLinkRejectsTraversalBeforeRepoint(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	current, other := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(current, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "live.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "api", current)

	if _, err := EnsureOwnedDirectoryLink(root, "../api", other, testOwnedDirectoryLinkOwner()); err == nil {
		t.Fatal("EnsureOwnedDirectoryLink accepted a traversal entry name")
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("traversal attempt disturbed original link = %q, %v", got, err)
	}
}

func TestEnsureOwnedDirectoryLinkKeepsOriginalLinkWhenReplacementTargetIsInvalid(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	current := t.TempDir()
	if err := os.WriteFile(filepath.Join(current, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "api", current)

	if _, err := EnsureOwnedDirectoryLink(root, "api", filepath.Join(t.TempDir(), "missing"), testOwnedDirectoryLinkOwner()); err == nil {
		t.Fatal("EnsureOwnedDirectoryLink accepted a missing replacement target")
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("failed replacement disturbed original link = %q, %v", got, err)
	}
}

func TestRestoreOwnedDirectoryLinkKeepsCurrentLinkWhenReplacementTargetIsInvalid(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	current := t.TempDir()
	if err := os.WriteFile(filepath.Join(current, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "api", current)

	if err := RestoreOwnedDirectoryLink(root, "api", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("RestoreOwnedDirectoryLink accepted a missing replacement target")
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("failed restore disturbed current link = %q, %v", got, err)
	}
}

func TestRenameInspectedDirectoryLinkRejectsChangedEntry(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	current, other := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(current, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "live.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedOwnedDirectoryLink(t, root, "api", current)
	inspected, err := os.Lstat(filepath.Join(root, "api"))
	if err != nil {
		t.Fatalf("Lstat(api): %v", err)
	}
	tempLink := filepath.Join(root, "api.tmp")
	if err := createPlatformDirectoryLink(other, tempLink); err != nil {
		t.Fatalf("create temp replacement link: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "api")); err != nil {
		t.Fatalf("remove original link: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatalf("create replacement directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "api", "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := renameInspectedDirectoryLink(tempLink, filepath.Join(root, "api"), inspected); err == nil {
		t.Fatal("renameInspectedDirectoryLink accepted a changed entry")
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "keep.txt")); err != nil || string(got) != "keep" {
		t.Fatalf("changed entry was disturbed = %q, %v", got, err)
	}
}

// A non-link entry (a real file or directory a reconcile did not create) is not
// Kandev's pointer to replace: it stays fail-closed and is never removed.
func TestEnsureOwnedDirectoryLinkRejectsNonLinkEntry(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	if err := os.MkdirAll(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "api", "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureOwnedDirectoryLink(root, "api", t.TempDir(), testOwnedDirectoryLinkOwner()); err == nil {
		t.Fatal("EnsureOwnedDirectoryLink overwrote a non-link entry")
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "keep.txt")); err != nil || string(got) != "keep" {
		t.Fatalf("non-link entry was disturbed = %q, %v", got, err)
	}

	// A regular file occupying the entry name is likewise not Kandev's pointer to
	// replace: it stays fail-closed and its bytes must survive untouched.
	file := filepath.Join(root, "web")
	if err := os.WriteFile(file, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureOwnedDirectoryLink(root, "web", t.TempDir(), testOwnedDirectoryLinkOwner()); err == nil {
		t.Fatal("EnsureOwnedDirectoryLink overwrote a non-link file")
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "keep" {
		t.Fatalf("non-link file was disturbed = %q, %v", got, err)
	}
}
