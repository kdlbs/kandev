package managedruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveNpxExecutionTreeDeletesOnlyExactTrustedKey(t *testing.T) {
	cacheRoot := t.TempDir()
	npxRoot := filepath.Join(cacheRoot, "_npx")
	if err := os.MkdirAll(npxRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	key := NpxExecutionCacheKey("@scope/managed-acp@1.2.3")
	target := filepath.Join(npxRoot, key)
	other := filepath.Join(npxRoot, "0123456789abcdef")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := RemoveNpxExecutionTree(cacheRoot, "@scope/managed-acp@1.2.3"); err != nil {
		t.Fatalf("RemoveNpxExecutionTree: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target stat error = %v, want not-exist", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated key was removed: %v", err)
	}
}

func TestRemoveNpxExecutionTreeIsIdempotentAndRejectsUnsafeTargets(t *testing.T) {
	cacheRoot := t.TempDir()
	if err := RemoveNpxExecutionTree(cacheRoot, "managed-acp@1.2.3"); err != nil {
		t.Fatalf("absent key should be idempotent: %v", err)
	}
	if err := RemoveNpxExecutionTree(cacheRoot, ""); err == nil {
		t.Fatal("empty package spec should be rejected")
	}
	if err := RemoveNpxExecutionTree(cacheRoot, "../escape"); err == nil {
		t.Fatal("path-like package spec should be rejected")
	}

	npxRoot := filepath.Join(cacheRoot, "_npx")
	if err := os.MkdirAll(npxRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(escape, []byte("do not delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := NpxExecutionCacheKey("managed-acp@1.2.3")
	if err := os.Symlink(escape, filepath.Join(npxRoot, key)); err != nil {
		t.Fatal(err)
	}
	if err := RemoveNpxExecutionTree(cacheRoot, "managed-acp@1.2.3"); err == nil {
		t.Fatal("symlinked execution tree should be rejected")
	}
	if _, err := os.Stat(escape); err != nil {
		t.Fatalf("symlink escape target was touched: %v", err)
	}

	regularFile := filepath.Join(npxRoot, NpxExecutionCacheKey("managed-acp@1.2.4"))
	if err := os.WriteFile(regularFile, []byte("not a tree"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveNpxExecutionTree(cacheRoot, "managed-acp@1.2.4"); err == nil {
		t.Fatal("regular-file execution tree should be rejected")
	}
	if _, err := os.Stat(regularFile); err != nil {
		t.Fatalf("regular-file execution tree was removed: %v", err)
	}
}

func TestRemoveNpxExecutionTreeRejectsCacheAndNpxRootSymlinks(t *testing.T) {
	cacheTarget := t.TempDir()
	cacheLink := filepath.Join(t.TempDir(), "cache-link")
	if err := os.Symlink(cacheTarget, cacheLink); err != nil {
		t.Fatal(err)
	}
	if err := RemoveNpxExecutionTree(cacheLink, "managed-acp@1.2.3"); err == nil {
		t.Fatal("symlinked npm cache root should be rejected")
	}

	cacheRoot := t.TempDir()
	npxTarget := t.TempDir()
	if err := os.Symlink(npxTarget, filepath.Join(cacheRoot, "_npx")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveNpxExecutionTree(cacheRoot, "managed-acp@1.2.3"); err == nil {
		t.Fatal("symlinked _npx root should be rejected")
	}
}
