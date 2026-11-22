package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeShimFixtureDir(t *testing.T, root, name string, withLock bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, githubCLIShimName()), []byte("shim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if withLock {
		if err := os.WriteFile(filepath.Join(dir, githubCLIShimLockName), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.21
func TestSweepStaleGitHubCLIShimDirsRemovesOnlyReleasedOwnedDirs(t *testing.T) {
	root := t.TempDir()
	stale := writeShimFixtureDir(t, root, githubCLIShimDirPrefix+"stale", true)
	legacy := writeShimFixtureDir(t, root, githubCLIShimDirPrefix+"legacy", false)
	unrelated := writeShimFixtureDir(t, root, "other-tool-1", true)

	sweepStaleGitHubCLIShimDirs(root)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale shim dir with a released lock remains: %v", err)
	}
	for _, kept := range []string{legacy, unrelated} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s must be kept: %v", kept, err)
		}
	}
}

// A Windows stop forcibly ends agentctl, so its own cleanup never runs. The
// next agentctl removes those directories, but never one whose owner is alive.
// @covers AC-EXECUTORS-SSH-WINDOWS-001.21
func TestSSHWindowsGitHubCLIShimSweepKeepsLiveOwner(t *testing.T) {
	if runtime.GOOS != windowsOS {
		t.Skip("relies on Windows refusing to delete an open file")
	}
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	liveDir, cleanup, err := installGitHubCLIShim(executable, root)
	if err != nil {
		t.Fatalf("install shim: %v", err)
	}
	stale := writeShimFixtureDir(t, root, githubCLIShimDirPrefix+"stale", true)

	sweepStaleGitHubCLIShimDirs(root)

	if _, err := os.Stat(filepath.Join(liveDir, githubCLIShimName())); err != nil {
		t.Fatalf("sweep removed a live owner's shim: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale shim dir remains: %v", err)
	}
	cleanup()
	if _, err := os.Stat(liveDir); !os.IsNotExist(err) {
		t.Fatalf("owner cleanup left its shim dir: %v", err)
	}
}
