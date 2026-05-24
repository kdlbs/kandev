package copyfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"only comma", ",", nil},
		{"single trimmed", " .env ", []string{".env"}},
		{"two", ".env,.env.local", []string{".env", ".env.local"}},
		{"dedupe", ".env, .env, .env.local", []string{".env", ".env.local"}},
		{"empties dropped", " .env , , .env.local ", []string{".env", ".env.local"}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Parse(tc.in)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	// WriteFile honors umask; force exact mode for predictable tests.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestCopy_LiteralFile(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, ".env"), "X=1", 0o600)

	_, warnings, err := Copy(context.Background(), src, dst, []string{".env"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	got := readFile(t, filepath.Join(dst, ".env"))
	if got != "X=1" {
		t.Fatalf("content = %q, want %q", got, "X=1")
	}

	info, err := os.Stat(filepath.Join(dst, ".env"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0o600", info.Mode().Perm())
	}
}

func TestCopy_Glob(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "a.local"), "A", 0o644)
	writeFile(t, filepath.Join(src, "b.local"), "B", 0o644)
	writeFile(t, filepath.Join(src, "c.txt"), "C", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, []string{"*.local"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if readFile(t, filepath.Join(dst, "a.local")) != "A" {
		t.Fatalf("a.local missing or wrong")
	}
	if readFile(t, filepath.Join(dst, "b.local")) != "B" {
		t.Fatalf("b.local missing or wrong")
	}
	if _, err := os.Stat(filepath.Join(dst, "c.txt")); !os.IsNotExist(err) {
		t.Fatalf("c.txt should not exist, err=%v", err)
	}
}

func TestCopy_DirectoryRecursive(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "config", "local.yml"), "y", 0o644)
	writeFile(t, filepath.Join(src, "config", "sub", "dev.json"), "j", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, []string{"config"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if readFile(t, filepath.Join(dst, "config", "local.yml")) != "y" {
		t.Fatalf("local.yml missing")
	}
	if readFile(t, filepath.Join(dst, "config", "sub", "dev.json")) != "j" {
		t.Fatalf("sub/dev.json missing")
	}
}

func TestCopy_NestedFile(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "config", "local.yml"), "y", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, []string{"config/local.yml"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if readFile(t, filepath.Join(dst, "config", "local.yml")) != "y" {
		t.Fatalf("nested file not copied")
	}
}

func TestCopy_MissingPattern(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	_, warnings, err := Copy(context.Background(), src, dst, []string{".env"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", warnings)
	}
	if !strings.Contains(warnings[0], ".env") {
		t.Fatalf("warning does not mention .env: %q", warnings[0])
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Fatalf("dst not empty: %v", entries)
	}
}

func TestCopy_GlobNoMatch(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "foo.txt"), "foo", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, []string{"*.local"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", warnings)
	}
}

func TestCopy_PathTraversal_Relative(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	dst := t.TempDir()

	// Create a file outside src
	writeFile(t, filepath.Join(parent, "escape.txt"), "leak", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, []string{"../escape.txt"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for traversal")
	}
	if _, err := os.Stat(filepath.Join(dst, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape.txt should not be copied")
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Fatalf("dst not empty: %v", entries)
	}
}

func TestCopy_PathTraversal_Absolute(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	dst := t.TempDir()

	outside := filepath.Join(parent, "abs_escape.txt")
	writeFile(t, outside, "leak", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, []string{outside}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for absolute traversal")
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Fatalf("dst not empty: %v", entries)
	}
}

func TestCopy_SymlinkInside(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often requires privilege on Windows")
	}
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, "real.env"), "REAL=1", 0o644)
	if err := os.Symlink("real.env", filepath.Join(src, ".env")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, warnings, err := Copy(context.Background(), src, dst, []string{".env"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	got := readFile(t, filepath.Join(dst, ".env"))
	if got != "REAL=1" {
		t.Fatalf("content = %q, want REAL=1", got)
	}

	// Confirm target is a regular file, not a symlink
	info, err := os.Lstat(filepath.Join(dst, ".env"))
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("target is symlink, want regular file")
	}
}

func TestCopy_SymlinkOutside(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often requires privilege on Windows")
	}
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	dst := t.TempDir()

	outside := filepath.Join(parent, "secret.txt")
	writeFile(t, outside, "SECRET", 0o644)

	if err := os.Symlink(outside, filepath.Join(src, "leak")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, warnings, err := Copy(context.Background(), src, dst, []string{"leak"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for symlink escaping src")
	}
	if _, err := os.Stat(filepath.Join(dst, "leak")); !os.IsNotExist(err) {
		t.Fatalf("leak should not be copied")
	}
}

func TestCopy_Idempotent(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, ".env"), "SRC", 0o644)
	writeFile(t, filepath.Join(dst, ".env"), "DST_ORIGINAL", 0o644)

	_, warnings, err := Copy(context.Background(), src, dst, []string{".env"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	got := readFile(t, filepath.Join(dst, ".env"))
	if got != "DST_ORIGINAL" {
		t.Fatalf("content = %q, want DST_ORIGINAL (existing file should not be overwritten)", got)
	}
}

func TestCopy_EmptyPatterns(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	for _, patterns := range [][]string{nil, {}} {
		_, warnings, err := Copy(context.Background(), src, dst, patterns, nil)
		if err != nil {
			t.Fatalf("Copy err: %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("warnings: %v", warnings)
		}
	}
}

func TestCopy_NilLogger(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, ".env"), "X", 0o644)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic with nil logger: %v", r)
		}
	}()

	if _, _, err := Copy(context.Background(), src, dst, []string{".env"}, nil); err != nil {
		t.Fatalf("Copy err: %v", err)
	}
}

func TestCopy_SymlinkedSourceDir(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink permission/semantics differ on Windows")
	}
	realDir := t.TempDir()
	linkedDir := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	writeFile(t, filepath.Join(realDir, ".env"), "X=1", 0o600)

	target := t.TempDir()
	_, warnings, err := Copy(context.Background(), linkedDir, target, []string{".env"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("should not reject symlinked source dir, warnings: %v", warnings)
	}
	got := readFile(t, filepath.Join(target, ".env"))
	if got != "X=1" {
		t.Fatalf("content = %q, want %q", got, "X=1")
	}
}

func TestCopy_MissingSourceDir(t *testing.T) {
	t.Parallel()
	_, _, err := Copy(context.Background(), "/nonexistent-dir-xyz", t.TempDir(), []string{".env"}, nil)
	if err == nil {
		t.Fatalf("expected error for missing source dir")
	}
}

func TestCopy_ContextCancelled(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, ".env"), "X", 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := Copy(ctx, src, dst, []string{".env"}, nil)
	if err == nil {
		t.Fatalf("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want wrapping context.Canceled", err)
	}
	if _, statErr := os.Stat(filepath.Join(dst, ".env")); !os.IsNotExist(statErr) {
		t.Fatalf("file should not be copied when ctx cancelled, statErr=%v", statErr)
	}
}

func TestPlan_HappyPath(t *testing.T) {
	t.Parallel()
	src := t.TempDir()

	writeFile(t, filepath.Join(src, ".env"), "X=1", 0o600)
	writeFile(t, filepath.Join(src, "config", "local.yml"), "y", 0o644)

	entries, warnings, err := Plan(context.Background(), src,
		[]string{".env", "config"}, nil)
	if err != nil {
		t.Fatalf("Plan err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.RelPath] = e
	}
	if string(got[".env"].Content) != "X=1" {
		t.Errorf(".env content = %q, want %q", got[".env"].Content, "X=1")
	}
	if got[".env"].Mode.Perm() != 0o600 {
		t.Errorf(".env mode = %v, want 0o600", got[".env"].Mode.Perm())
	}
	if string(got["config/local.yml"].Content) != "y" {
		t.Errorf("config/local.yml content = %q, want %q", got["config/local.yml"].Content, "y")
	}
}

func TestPlan_OversizedFileSkipped(t *testing.T) {
	t.Parallel()
	src := t.TempDir()

	// Write a payload exceeding MaxEntryBytes (5MB). Use a sparse-ish
	// buffer to avoid stressing the test runner's RAM.
	huge := make([]byte, MaxEntryBytes+1)
	writeFile(t, filepath.Join(src, "big.bin"), string(huge), 0o644)
	writeFile(t, filepath.Join(src, ".env"), "OK=1", 0o644)

	entries, warnings, err := Plan(context.Background(), src,
		[]string{"big.bin", ".env"}, nil)
	if err != nil {
		t.Fatalf("Plan err: %v", err)
	}
	if len(entries) != 1 || entries[0].RelPath != ".env" {
		t.Fatalf("entries = %+v, expected just .env", entries)
	}
	foundCapWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "big.bin") && strings.Contains(w, "cap") {
			foundCapWarning = true
		}
	}
	if !foundCapWarning {
		t.Fatalf("expected cap warning for big.bin, got %v", warnings)
	}
}

func TestPlan_PathTraversalRejected(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	src := filepath.Join(parent, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(parent, "escape.txt"), "leak", 0o644)

	entries, warnings, err := Plan(context.Background(), src,
		[]string{"../escape.txt"}, nil)
	if err != nil {
		t.Fatalf("Plan err: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected zero entries for traversal, got %+v", entries)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for traversal")
	}
}

func TestWriteEntries_HappyPath(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()

	entries := []Entry{
		{RelPath: ".env", Mode: 0o600, Content: []byte("X=1")},
		{RelPath: "config/local.yml", Mode: 0o644, Content: []byte("y")},
	}

	copied, warnings, err := WriteEntries(context.Background(), dst, entries, nil)
	if err != nil {
		t.Fatalf("WriteEntries err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(copied) != 2 {
		t.Fatalf("copied = %v, want 2 entries", copied)
	}
	if got := readFile(t, filepath.Join(dst, ".env")); got != "X=1" {
		t.Errorf(".env content = %q", got)
	}
	info, err := os.Stat(filepath.Join(dst, ".env"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf(".env mode = %v, want 0o600", info.Mode().Perm())
	}
}

func TestWriteEntries_SkipIfExists(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	writeFile(t, filepath.Join(dst, ".env"), "PREEXISTING", 0o644)

	copied, _, err := WriteEntries(context.Background(), dst,
		[]Entry{{RelPath: ".env", Mode: 0o600, Content: []byte("NEW")}}, nil)
	if err != nil {
		t.Fatalf("WriteEntries err: %v", err)
	}
	if len(copied) != 0 {
		t.Errorf("copied = %v, want empty (skip-if-exists)", copied)
	}
	if got := readFile(t, filepath.Join(dst, ".env")); got != "PREEXISTING" {
		t.Errorf("existing file overwritten: %q", got)
	}
}

func TestWriteEntries_RejectsSymlinkedParentEscape(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation often requires privilege on Windows")
	}

	parent := t.TempDir()
	dst := filepath.Join(parent, "dst")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	// A pre-existing symlinked directory inside the target — `dst/config`
	// points at a directory completely outside the target root. Writing
	// `config/sneaky.txt` via lexical join lands at `outside/sneaky.txt`
	// before the EvalSymlinks guard was added.
	if err := os.Symlink(outside, filepath.Join(dst, "config")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	copied, warnings, err := WriteEntries(context.Background(), dst,
		[]Entry{{RelPath: "config/sneaky.txt", Mode: 0o644, Content: []byte("PWN")}}, nil)
	if err != nil {
		t.Fatalf("WriteEntries err: %v", err)
	}
	if len(copied) != 0 {
		t.Errorf("copied = %v, want zero (escape rejected)", copied)
	}
	if len(warnings) == 0 {
		t.Error("expected warning for symlinked-parent escape")
	}
	if _, err := os.Stat(filepath.Join(outside, "sneaky.txt")); !os.IsNotExist(err) {
		t.Errorf("sneaky.txt should NOT exist outside dst; stat err = %v", err)
	}
}

func TestWriteEntries_RejectsTraversalEntries(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	dst := filepath.Join(parent, "dst")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	bad := []Entry{
		{RelPath: "../escape.txt", Mode: 0o644, Content: []byte("leak")},
		{RelPath: "/etc/passwd", Mode: 0o644, Content: []byte("leak")},
		{RelPath: "", Mode: 0o644, Content: []byte("x")},
	}
	copied, warnings, err := WriteEntries(context.Background(), dst, bad, nil)
	if err != nil {
		t.Fatalf("WriteEntries err: %v", err)
	}
	if len(copied) != 0 {
		t.Errorf("copied = %v, want zero (all rejected)", copied)
	}
	if len(warnings) != 3 {
		t.Errorf("warnings = %v, want 3", warnings)
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !os.IsNotExist(err) {
		t.Errorf("escape.txt should not exist outside dst")
	}
}

func TestCopy_ReturnsCopiedRelPaths(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, filepath.Join(src, ".env"), "X=1", 0o644)
	writeFile(t, filepath.Join(src, "config", "local.yml"), "y", 0o644)
	writeFile(t, filepath.Join(src, "config", "sub", "dev.json"), "j", 0o644)
	// Pre-existing dst file should be skipped and NOT appear in copied.
	writeFile(t, filepath.Join(src, "already.txt"), "src", 0o644)
	writeFile(t, filepath.Join(dst, "already.txt"), "preexisting", 0o644)

	copied, warnings, err := Copy(context.Background(), src, dst,
		[]string{".env", "config", "already.txt"}, nil)
	if err != nil {
		t.Fatalf("Copy err: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	got := map[string]bool{}
	for _, p := range copied {
		got[p] = true
	}
	want := []string{".env", "config/local.yml", "config/sub/dev.json"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("expected copied list to contain %q, got %v", w, copied)
		}
	}
	if got["already.txt"] {
		t.Errorf("skip-if-exists entry should NOT appear in copied list, got %v", copied)
	}
	if len(copied) != len(want) {
		t.Errorf("copied len = %d, want %d (list: %v)", len(copied), len(want), copied)
	}
}
