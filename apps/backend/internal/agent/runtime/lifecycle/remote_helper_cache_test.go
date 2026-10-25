package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneRemoteHelperCacheKeepsCurrentAndTwoPreviousVersions(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0", "1.4.0"} {
		writeCachedHelperVersion(t, root, version)
	}

	removed, err := PruneRemoteHelperCache(root, "1.4.0", nil, true)
	if err != nil {
		t.Fatalf("PruneRemoteHelperCache: %v", err)
	}
	if len(removed) != 2 || removed[0] != "1.1.0" || removed[1] != "1.0.0" {
		t.Fatalf("removed versions = %v, want [1.1.0 1.0.0]", removed)
	}
	for _, version := range []string{"1.2.0", "1.3.0", "1.4.0"} {
		if _, err := os.Stat(filepath.Join(root, version)); err != nil {
			t.Fatalf("retained %s: %v", version, err)
		}
	}
}

func TestPruneRemoteHelperCacheProtectsMountedVersionAndDefersUnknownInventory(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0", "1.4.0"} {
		writeCachedHelperVersion(t, root, version)
	}
	pinned := filepath.Join(root, "1.0.0", "linux-amd64", "digest", "agentctl")
	if removed, err := PruneRemoteHelperCache(root, "1.4.0", []string{pinned}, false); err != nil || len(removed) != 0 {
		t.Fatalf("uncertain inventory prune = %v, %v; want no removal", removed, err)
	}
	removed, err := PruneRemoteHelperCache(root, "1.4.0", []string{pinned}, true)
	if err != nil {
		t.Fatalf("PruneRemoteHelperCache: %v", err)
	}
	if len(removed) != 1 || removed[0] != "1.1.0" {
		t.Fatalf("removed versions = %v, want only unpinned 1.1.0", removed)
	}
	if _, err := os.Stat(filepath.Join(root, "1.0.0")); err != nil {
		t.Fatalf("pinned version removed: %v", err)
	}
}

func TestPruneRemoteHelperCacheSupportsRollbackWindow(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0", "1.4.0"} {
		writeCachedHelperVersion(t, root, version)
	}

	if _, err := PruneRemoteHelperCache(root, "1.2.0", nil, true); err != nil {
		t.Fatalf("rollback prune: %v", err)
	}
	for _, version := range []string{"1.0.0", "1.1.0", "1.2.0"} {
		if _, err := os.Stat(filepath.Join(root, version)); err != nil {
			t.Fatalf("rollback version %s was not retained: %v", version, err)
		}
	}
	for _, version := range []string{"1.3.0", "1.4.0"} {
		if _, err := os.Stat(filepath.Join(root, version)); !os.IsNotExist(err) {
			t.Fatalf("future version %s remains after rollback cleanup: %v", version, err)
		}
	}
}

func writeCachedHelperVersion(t *testing.T, root, version string) {
	t.Helper()
	path := filepath.Join(root, version, "linux-amd64", "digest", "agentctl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(version), 0o755); err != nil {
		t.Fatal(err)
	}
}
