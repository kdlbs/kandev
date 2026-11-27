//go:build windows

package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverySnapshotPreservesWindowsReadOnlyMode(t *testing.T) {
	source := t.TempDir()
	filePath := filepath.Join(source, "readonly")
	if err := os.WriteFile(filePath, []byte("recovery mode fixture\n"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.Chmod(filePath, 0o444); err != nil {
		t.Fatalf("set source read-only mode: %v", err)
	}
	sourceInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat source file: %v", err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := snapshotCheckout(source, snapshot); err != nil {
		t.Fatalf("snapshotCheckout: %v", err)
	}
	assertWindowsRecoveryMode(t, filepath.Join(snapshot, "readonly"), sourceInfo.Mode().Perm())

	replacement := t.TempDir()
	if err := copySnapshotEntries(snapshot, replacement); err != nil {
		t.Fatalf("copySnapshotEntries: %v", err)
	}
	assertWindowsRecoveryMode(t, filepath.Join(replacement, "readonly"), sourceInfo.Mode().Perm())
}

func assertWindowsRecoveryMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %q = %04o, want %04o", path, got, want)
	}
}
