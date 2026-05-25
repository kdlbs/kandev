// Tests for the per-subdirectory disk walker. Covers happy-path sizing
// across all configured subdirs and graceful handling of a chmod-000
// subdirectory (which should produce a warning string while still totalling
// the readable siblings).
package disk

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFile creates parents and a regular file with the given content.
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestWalkSubdir_MissingRootReturnsZero(t *testing.T) {
	res := walkSubdir(filepath.Join(t.TempDir(), "does-not-exist"))
	if res.bytes != 0 {
		t.Errorf("expected 0 bytes for missing root, got %d", res.bytes)
	}
	if len(res.warnings) != 0 {
		t.Errorf("expected no warnings for missing root, got %v", res.warnings)
	}
}

func TestWalkSubdir_SumsRegularFilesRecursively(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), []byte("hello"))                   // 5
	writeFile(t, filepath.Join(root, "nested", "b.txt"), []byte("world!"))        // 6
	writeFile(t, filepath.Join(root, "nested", "deep", "c.bin"), []byte("12345")) // 5

	res := walkSubdir(root)
	const want = int64(16)
	if res.bytes != want {
		t.Errorf("bytes = %d, want %d", res.bytes, want)
	}
	if len(res.warnings) != 0 {
		t.Errorf("expected no warnings, got %v", res.warnings)
	}
}

func TestWalkSubdir_RecordsPermissionWarningAndContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission test is POSIX only")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}

	root := t.TempDir()
	// Readable sibling contributes a known size.
	writeFile(t, filepath.Join(root, "readable", "ok.txt"), []byte("abcdef")) // 6

	// Unreadable sibling: file inside must exist before chmod so WalkDir
	// reports the permission error when descending.
	blocked := filepath.Join(root, "blocked")
	writeFile(t, filepath.Join(blocked, "secret.txt"), []byte("xx"))
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatalf("chmod blocked: %v", err)
	}
	t.Cleanup(func() {
		// Restore permissions so the temp dir can be cleaned up.
		_ = os.Chmod(blocked, 0o755)
	})

	res := walkSubdir(root)

	if res.bytes != 6 {
		t.Errorf("bytes = %d, want 6 (readable sibling only)", res.bytes)
	}
	if len(res.warnings) == 0 {
		t.Fatalf("expected at least one warning for the chmod-000 subdir")
	}
	joined := strings.Join(res.warnings, "\n")
	if !strings.Contains(joined, "blocked") {
		t.Errorf("expected warning to mention the blocked subdir, got %q", joined)
	}
}

func TestSubdirsFor_CoversAllExpectedBuckets(t *testing.T) {
	got := subdirsFor("/tmp/example")
	wantNames := []string{"data_dir", "worktrees", "repos", "sessions", "tasks", "quick_chat", "backups"}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d subdirs, want %d", len(got), len(wantNames))
	}
	for i, sd := range got {
		if sd.name != wantNames[i] {
			t.Errorf("subdir[%d].name = %s, want %s", i, sd.name, wantNames[i])
		}
	}
	// Backups must live under data/, not at the home root.
	for _, sd := range got {
		if sd.name == "backups" && !strings.HasSuffix(sd.path, filepath.Join("data", "backups")) {
			t.Errorf("backups path = %s, want suffix data/backups", sd.path)
		}
	}
}
