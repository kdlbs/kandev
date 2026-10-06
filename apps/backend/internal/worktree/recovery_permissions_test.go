//go:build unix

package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverySnapshotPreservesSupportedModes(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	writeRecoveryModeFixture(t, filepath.Join(source, "group-writable"), 0o775)
	writeRecoveryModeFixture(t, filepath.Join(source, "nested", "child"), 0o670)
	if err := os.Chmod(filepath.Join(source, "nested"), 0o711); err != nil {
		t.Fatalf("set source directory mode: %v", err)
	}

	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := snapshotCheckout(source, snapshot); err != nil {
		t.Fatalf("snapshotCheckout: %v", err)
	}
	assertRecoveryMode(t, filepath.Join(snapshot, "group-writable"), 0o775)
	assertRecoveryMode(t, filepath.Join(snapshot, "nested"), 0o711)
	assertRecoveryMode(t, filepath.Join(snapshot, "nested", "child"), 0o670)
}

func TestRecoveryRestorePreservesSupportedModes(t *testing.T) {
	snapshot := t.TempDir()
	if err := os.Mkdir(filepath.Join(snapshot, "nested"), 0o700); err != nil {
		t.Fatalf("create snapshot directory: %v", err)
	}
	writeRecoveryModeFixture(t, filepath.Join(snapshot, "group-writable"), 0o775)
	writeRecoveryModeFixture(t, filepath.Join(snapshot, "nested", "child"), 0o670)
	if err := os.Chmod(filepath.Join(snapshot, "nested"), 0o711); err != nil {
		t.Fatalf("set snapshot directory mode: %v", err)
	}

	replacement := t.TempDir()
	if err := copySnapshotEntries(snapshot, replacement); err != nil {
		t.Fatalf("copySnapshotEntries: %v", err)
	}
	assertRecoveryMode(t, filepath.Join(replacement, "group-writable"), 0o775)
	assertRecoveryMode(t, filepath.Join(replacement, "nested"), 0o711)
	assertRecoveryMode(t, filepath.Join(replacement, "nested", "child"), 0o670)
}

func writeRecoveryModeFixture(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("recovery mode fixture\n"), 0o600); err != nil {
		t.Fatalf("write fixture %q: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set fixture mode %q: %v", path, err)
	}
}

func assertRecoveryMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want.Perm() {
		t.Fatalf("mode for %q = %04o, want %04o", path, got, want.Perm())
	}
}
