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

	if err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env.local", Mode: FileMaterializeCopy}}); err != nil {
		t.Fatalf("MaterializeWorktreeFiles: %v", err)
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

	if err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env.local", Mode: FileMaterializeSymlink}}); err != nil {
		t.Fatalf("MaterializeWorktreeFiles: %v", err)
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
	if err := MaterializeWorktreeFiles(src, dest, files); err != nil {
		t.Fatalf("MaterializeWorktreeFiles: %v", err)
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

	if err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env"}}); err != nil {
		t.Fatalf("MaterializeWorktreeFiles: %v", err)
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
	if err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env.local", Mode: FileMaterializeCopy}}); err != nil {
		t.Fatalf("MaterializeWorktreeFiles: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("expected dest not to exist, got err=%v", err)
	}
}

func TestMaterializeWorktreeFiles_NestedPath(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "config", "secrets.env"), "K=V")

	if err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: "config/secrets.env", Mode: FileMaterializeCopy}}); err != nil {
		t.Fatalf("MaterializeWorktreeFiles: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "config", "secrets.env"))
	if err != nil {
		t.Fatalf("read nested dest: %v", err)
	}
	if string(content) != "K=V" {
		t.Fatalf("nested content = %q", content)
	}
}

func TestMaterializeWorktreeFiles_RejectsTraversal(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, "outside.txt"), "nope")

	for _, bad := range []string{"../outside.txt", "/etc/passwd"} {
		err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: bad, Mode: FileMaterializeCopy}})
		if err == nil {
			t.Fatalf("expected error for path %q, got nil", bad)
		}
	}
}

func TestMaterializeWorktreeFiles_EmptyListNoop(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	if err := MaterializeWorktreeFiles(src, dest, nil); err != nil {
		t.Fatalf("empty list should be a no-op, got %v", err)
	}
	// Blank / whitespace entries are ignored.
	if err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ""}, {Path: "   "}}); err != nil {
		t.Fatalf("blank entries should be ignored, got %v", err)
	}
}

func TestMaterializeWorktreeFiles_SymlinkReplacesExisting(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeFile(t, filepath.Join(src, ".env"), "FROM=src")
	// A stale copy already sits at the destination.
	writeFile(t, filepath.Join(dest, ".env"), "FROM=stale")

	if err := MaterializeWorktreeFiles(src, dest, []FileSpec{{Path: ".env", Mode: FileMaterializeSymlink}}); err != nil {
		t.Fatalf("MaterializeWorktreeFiles: %v", err)
	}
	info, err := os.Lstat(filepath.Join(dest, ".env"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected existing file to be replaced with a symlink")
	}
}
