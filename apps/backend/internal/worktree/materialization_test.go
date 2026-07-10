package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestNormalizeFileMaterializeMode(t *testing.T) {
	cases := []struct {
		in   string
		want FileMaterializeMode
	}{
		{"", FileMaterializeCopy},
		{"copy", FileMaterializeCopy},
		{"COPY", FileMaterializeCopy},
		{"symlink", FileMaterializeSymlink},
		{"  symlink  ", FileMaterializeSymlink}, // surrounding whitespace is trimmed
		{"bogus", FileMaterializeCopy},          // unknown normalizes to the safe default
	}
	for _, tc := range cases {
		if got := NormalizeFileMaterializeMode(tc.in); got != tc.want {
			t.Errorf("NormalizeFileMaterializeMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateFileMaterializeMode(t *testing.T) {
	valid := []string{"", "copy", "symlink", "COPY", " Symlink "}
	for _, v := range valid {
		if err := ValidateFileMaterializeMode(v); err != nil {
			t.Errorf("ValidateFileMaterializeMode(%q) unexpected error: %v", v, err)
		}
	}
	invalid := []string{"link", "hardlink", "move", "cp"}
	for _, v := range invalid {
		if err := ValidateFileMaterializeMode(v); !errors.Is(err, ErrInvalidFileMaterializeMode) {
			t.Errorf("ValidateFileMaterializeMode(%q) = %v, want ErrInvalidFileMaterializeMode", v, err)
		}
	}
}

func TestMaterializeWorktreeFiles_CopyMode(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, ".env.local"), "SECRET=1")

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env.local", Mode: FileMaterializeCopy}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}

	destFile := filepath.Join(dest, ".env.local")
	info, err := os.Lstat(destFile)
	if err != nil {
		t.Fatalf("lstat dest: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("copy mode produced a symlink, want a regular file")
	}
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(content) != "SECRET=1" {
		t.Fatalf("dest content = %q, want %q", content, "SECRET=1")
	}

	// Copy is an isolated snapshot: mutating the source must not change the copy.
	writeFile(t, filepath.Join(src, ".env.local"), "SECRET=changed")
	content, _ = os.ReadFile(destFile)
	if string(content) != "SECRET=1" {
		t.Fatalf("copy leaked source mutation: got %q", content)
	}
}

func TestMaterializeWorktreeFiles_SymlinkMode(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, ".env.local"), "SHARED=1")

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env.local", Mode: FileMaterializeSymlink}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}

	destFile := filepath.Join(dest, ".env.local")
	info, err := os.Lstat(destFile)
	if err != nil {
		t.Fatalf("lstat dest: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink mode did not produce a symlink")
	}

	// The link must resolve to the source content...
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("read through symlink: %v", err)
	}
	if string(content) != "SHARED=1" {
		t.Fatalf("symlink content = %q, want %q", content, "SHARED=1")
	}

	// ...and stay in sync when the central file changes.
	writeFile(t, filepath.Join(src, ".env.local"), "SHARED=updated")
	content, _ = os.ReadFile(destFile)
	if string(content) != "SHARED=updated" {
		t.Fatalf("symlink did not reflect source update: got %q", content)
	}
}

func TestMaterializeWorktreeFiles_MixedPerFileModes(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, ".env.local"), "COPIED=1")
	writeFile(t, filepath.Join(src, ".env.shared"), "LINKED=1")

	files := []FileSpec{
		{Path: ".env.local", Mode: FileMaterializeCopy},
		{Path: ".env.shared", Mode: FileMaterializeSymlink},
	}
	if w := MaterializeWorktreeFiles(src, dest, files); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}

	copied, err := os.Lstat(filepath.Join(dest, ".env.local"))
	if err != nil {
		t.Fatalf("lstat copied: %v", err)
	}
	if copied.Mode()&os.ModeSymlink != 0 {
		t.Fatalf(".env.local should be a copy, got a symlink")
	}

	linked, err := os.Lstat(filepath.Join(dest, ".env.shared"))
	if err != nil {
		t.Fatalf("lstat linked: %v", err)
	}
	if linked.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".env.shared should be a symlink, got a regular file")
	}
}

// An empty per-file mode defaults to copy.
func TestMaterializeWorktreeFiles_EmptyModeDefaultsToCopy(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, ".env"), "X=1")

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env"}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}
	info, err := os.Lstat(filepath.Join(dest, ".env"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("empty mode should default to copy, got a symlink")
	}
}

func TestMaterializeWorktreeFiles_MissingSourceSkipped(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	// No source file exists; a missing configured file is benign and skipped.
	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env.local", Mode: FileMaterializeCopy}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}
	if _, err := os.Lstat(filepath.Join(dest, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("expected dest not to exist, got err=%v", err)
	}
}

func TestMaterializeWorktreeFiles_NestedPath(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "config", "secrets.env"), "K=V")

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: "config/secrets.env", Mode: FileMaterializeCopy}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}
	content, err := os.ReadFile(filepath.Join(dest, "config", "secrets.env"))
	if err != nil {
		t.Fatalf("read nested dest: %v", err)
	}
	if string(content) != "K=V" {
		t.Fatalf("nested content = %q", content)
	}
}

// CleanWorktreeFilePath enforces the path-integrity contract: absolute,
// traversal, and reserved .git paths return ErrInvalidWorktreeFilePath.
func TestCleanWorktreeFilePath_RejectsInvalid(t *testing.T) {
	for _, bad := range []string{"../outside.txt", "/etc/passwd", ".git", ".git/config", "sub/../../escape"} {
		if _, err := CleanWorktreeFilePath(bad); !errors.Is(err, ErrInvalidWorktreeFilePath) {
			t.Errorf("CleanWorktreeFilePath(%q) = %v, want ErrInvalidWorktreeFilePath", bad, err)
		}
	}
	for _, ok := range []string{".env", "config/secrets.env", "a/b/c.txt"} {
		if _, err := CleanWorktreeFilePath(ok); err != nil {
			t.Errorf("CleanWorktreeFilePath(%q) unexpected error: %v", ok, err)
		}
	}
}

func TestMaterializeWorktreeFiles_RejectsTraversal(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "outside.txt"), "nope")

	for _, bad := range []string{"../outside.txt", "/etc/passwd"} {
		w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: bad, Mode: FileMaterializeCopy}})
		if len(w) == 0 {
			t.Fatalf("expected a warning for path %q, got none", bad)
		}
	}
}

// The reserved .git admin path must never be materialized (it would clobber the
// worktree's git metadata via os.RemoveAll on the destination).
func TestMaterializeWorktreeFiles_RejectsGitPath(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, ".git", "config"), "x")

	for _, bad := range []string{".git", ".git/config"} {
		w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: bad, Mode: FileMaterializeCopy}})
		if len(w) == 0 {
			t.Fatalf("path %q: expected a warning, got none", bad)
		}
		if _, statErr := os.Lstat(filepath.Join(dest, bad)); !os.IsNotExist(statErr) {
			t.Fatalf("reserved path %q was materialized", bad)
		}
	}
}

// A file configured under a symlinked destination ancestor must be rejected so
// materialization can't escape the worktree via os.MkdirAll following the link.
func TestMaterializeWorktreeFiles_RejectsSymlinkedDestAncestor(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(src, "shared", "app.env"), "K=V")

	// Make dest/shared a symlink to a directory outside the worktree.
	if err := os.Symlink(outside, filepath.Join(dest, "shared")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: "shared/app.env", Mode: FileMaterializeCopy}})
	if len(w) == 0 {
		t.Fatalf("expected a warning for symlinked ancestor, got none")
	}
	// Nothing should have been written through the symlink into the outside dir.
	if _, statErr := os.Lstat(filepath.Join(outside, "app.env")); !os.IsNotExist(statErr) {
		t.Fatalf("file escaped through symlinked ancestor: %v", statErr)
	}
}

// Copy mode must not follow a repo-controlled symlink whose target resolves
// outside the repository (exfiltration guard).
func TestMaterializeWorktreeFiles_CopyRejectsSymlinkEscapingRepo(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret"), "TOP SECRET")
	// A repo-tracked symlink pointing outside the repo, e.g. .env -> ~/.ssh/id_rsa.
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(src, ".env")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env", Mode: FileMaterializeCopy}})
	if len(w) == 0 {
		t.Fatalf("expected a warning for symlink escaping repo, got none")
	}
	if _, statErr := os.Lstat(filepath.Join(dest, ".env")); !os.IsNotExist(statErr) {
		t.Fatalf("symlinked-out source was materialized into the worktree")
	}
}

// One failing entry must not stop the others; failures surface as warnings.
func TestMaterializeWorktreeFiles_ContinuesPastFailures(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "good.env"), "OK=1")

	w := MaterializeWorktreeFiles(src, dest, []FileSpec{
		{Path: "../escape", Mode: FileMaterializeCopy}, // invalid → warning
		{Path: "good.env", Mode: FileMaterializeCopy},  // still materialized
	})
	if len(w) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(w), w)
	}
	if _, err := os.Lstat(filepath.Join(dest, "good.env")); err != nil {
		t.Fatalf("later file not materialized after an earlier failure: %v", err)
	}
}

// copyDir must recreate symlinks inside a copied directory rather than following
// them (which would EISDIR on symlinked dirs or copy with wrong perms).
func TestMaterializeWorktreeFiles_CopyDirPreservesInnerSymlink(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "cfg", "real.env"), "K=V")
	if err := os.Symlink("real.env", filepath.Join(src, "cfg", "link.env")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: "cfg", Mode: FileMaterializeCopy}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}
	info, err := os.Lstat(filepath.Join(dest, "cfg", "link.env"))
	if err != nil {
		t.Fatalf("lstat copied link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("inner symlink was not preserved as a symlink")
	}
}

// A configured path that is a symlink to a directory must copy the directory's
// contents (copy semantics), not be recreated as a symlink.
func TestMaterializeWorktreeFiles_CopySymlinkedDirCopiesContents(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "real", "app.env"), "K=V")
	// "cfg" is a symlink to the real directory, both inside the repo.
	if err := os.Symlink("real", filepath.Join(src, "cfg")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: "cfg", Mode: FileMaterializeCopy}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}
	info, err := os.Lstat(filepath.Join(dest, "cfg"))
	if err != nil {
		t.Fatalf("lstat copied dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("symlinked directory was not copied as a real directory (mode %s)", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(dest, "cfg", "app.env")); err != nil {
		t.Fatalf("directory contents not copied: %v", err)
	}
}

// A symlink nested inside a copied directory whose target escapes the repository
// must be skipped, not recreated, so the worktree never points outside the repo.
func TestMaterializeWorktreeFiles_CopyDirSkipsEscapingInnerSymlink(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret"), "TOP SECRET")
	writeFile(t, filepath.Join(src, "cfg", "real.env"), "K=V")
	// An inner symlink pointing outside the repository.
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(src, "cfg", "escape")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: "cfg", Mode: FileMaterializeCopy}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}
	// The safe inner file is copied, the escaping link is not recreated.
	if _, err := os.Stat(filepath.Join(dest, "cfg", "real.env")); err != nil {
		t.Fatalf("safe inner file not copied: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "cfg", "escape")); !os.IsNotExist(err) {
		t.Fatalf("escaping inner symlink was materialized: %v", err)
	}
}

func TestMaterializeWorktreeFiles_EmptyListNoop(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	if w := MaterializeWorktreeFiles(src, dest, nil); len(w) != 0 {
		t.Fatalf("empty list should be a no-op, got %v", w)
	}
	// Blank / whitespace entries are ignored.
	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ""}, {Path: "   "}}); len(w) != 0 {
		t.Fatalf("blank entries should be ignored, got %v", w)
	}
}

func TestMaterializeWorktreeFiles_SymlinkReplacesExisting(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, ".env"), "FROM=src")
	// A stale copy already sits at the destination.
	writeFile(t, filepath.Join(dest, ".env"), "FROM=stale")

	if w := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env", Mode: FileMaterializeSymlink}}); len(w) != 0 {
		t.Fatalf("MaterializeWorktreeFiles: %v", w)
	}
	info, err := os.Lstat(filepath.Join(dest, ".env"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected existing file to be replaced with a symlink")
	}
}
