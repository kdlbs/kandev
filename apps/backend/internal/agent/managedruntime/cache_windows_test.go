//go:build windows

package managedruntime

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3
func TestRemoveNpxExecutionTreeTreatsMissingWindowsChildAsAbsent(t *testing.T) {
	cacheRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(cacheRoot, "_npx"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := RemoveNpxExecutionTree(cacheRoot, "managed-acp@1.2.3"); err != nil {
		t.Fatalf("absent execution tree should be idempotent: %v", err)
	}
}

func createManagedRuntimeJunction(t *testing.T, link, target string) {
	t.Helper()
	command := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create junction %s -> %s: %v\n%s", link, target, err, output)
	}
	if _, err := os.Stat(link); err != nil {
		t.Fatalf("junction %s does not resolve to its target: %v", link, err)
	}
}

// The cache root is opened through the shared no-follow walk: the
// object-manager volume link is resolved once and every user-controlled
// component below it is opened relative to its parent.
func TestOpenManagedRuntimeDirectoryPathWalksPlainCacheRoot(t *testing.T) {
	cacheRoot := t.TempDir()
	nested := filepath.Join(cacheRoot, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested cache root: %v", err)
	}

	handle, err := openManagedRuntimeDirectoryPath(nested)
	if err != nil {
		t.Fatalf("open plain cache root: %v", err)
	}
	_ = windows.CloseHandle(handle)
}

func TestOpenManagedRuntimeDirectoryPathRejectsJunctions(t *testing.T) {
	cacheRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir outside sub: %v", err)
	}
	junction := filepath.Join(cacheRoot, "linked")
	createManagedRuntimeJunction(t, junction, outside)

	t.Run("junction ancestor of the root", func(t *testing.T) {
		handle, err := openManagedRuntimeDirectoryPath(filepath.Join(junction, "sub"))
		if err == nil {
			_ = windows.CloseHandle(handle)
			t.Fatal("open followed a junction ancestor of the cache root")
		}
	})

	t.Run("root is the junction", func(t *testing.T) {
		handle, err := openManagedRuntimeDirectoryPath(junction)
		if err == nil {
			_ = windows.CloseHandle(handle)
			t.Fatal("open followed a junction as the cache root")
		}
	})
}

// Removing an execution tree through a junction must fail and must not touch
// the directory the junction points at.
func TestRemoveNpxExecutionTreeRejectsJunction(t *testing.T) {
	cacheRoot := t.TempDir()
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	npxRoot := filepath.Join(cacheRoot, "_npx")
	if err := os.Mkdir(npxRoot, 0o755); err != nil {
		t.Fatalf("mkdir _npx: %v", err)
	}
	createManagedRuntimeJunction(t, filepath.Join(npxRoot, NpxExecutionCacheKey("managed-acp@1.2.3")), outside)

	if err := RemoveNpxExecutionTree(cacheRoot, "managed-acp@1.2.3"); err == nil {
		t.Fatal("RemoveNpxExecutionTree removed a tree through a junction")
	}
	content, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel removed through the junction: %v", err)
	}
	if string(content) != "keep" {
		t.Fatalf("sentinel content = %q, want %q", content, "keep")
	}
}
