//go:build windows

package workspaces

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// openShareIncompatibleHolder opens dir the way Windows opens a process current
// directory: FILE_SHARE_READ|FILE_SHARE_WRITE without FILE_SHARE_DELETE. Any
// later open that requests DELETE access on the same directory then fails with
// STATUS_SHARING_VIOLATION, which is the exact condition that broke worktree
// resume. The observed holders were a stale shell whose CWD was the task root
// and, more commonly, Kandev's own Terminal-panel shell (working_dir = task
// root); either makes an unpatched Resume fail after an agent error.
func openShareIncompatibleHolder(t *testing.T, dir string) windows.Handle {
	t.Helper()
	name, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatalf("utf16 path: %v", err)
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		t.Fatalf("open share-incompatible holder for %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
	return handle
}

func writeWorktreeGitFile(t *testing.T, target string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(target, ".git"), []byte("gitdir: linked\n"), 0o600); err != nil {
		t.Fatalf("write .git: %v", err)
	}
}

func TestOpenDirectoryNoFollowSucceedsWithShareIncompatibleHolder(t *testing.T) {
	taskRoot := t.TempDir()
	worktree := filepath.Join(taskRoot, "repo")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	writeWorktreeGitFile(t, worktree)

	// A stale process whose current directory is the task root and one whose
	// current directory is the worktree itself: both hold DELETE-incompatible
	// handles, matching the reported repro.
	openShareIncompatibleHolder(t, taskRoot)
	openShareIncompatibleHolder(t, worktree)

	handle, err := OpenDirectoryNoFollow(taskRoot, worktree)
	if err != nil {
		t.Fatalf("OpenDirectoryNoFollow with DELETE-incompatible holders: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	if !handle.IsValidWorktree() {
		t.Fatal("IsValidWorktree returned false through the validation handle")
	}
	if err := handle.VerifyPath(worktree); err != nil {
		t.Fatalf("VerifyPath through the validation handle: %v", err)
	}

	// Guard the root cause: requesting DELETE against such a holder is exactly
	// what used to fail resume, so validation must not request it.
	deleteHandle, err := openWindowsDependencyDirectoryPath(worktree, windowsDependencyDeleteAccess)
	if err == nil {
		_ = windows.CloseHandle(deleteHandle)
		t.Fatal("expected a DELETE-access open to fail against a share-incompatible holder")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("DELETE-access open error = %v, want sharing violation", err)
	}
}

func TestRemoveDirectoryThroughValidationHandleDeletesWhenUnheld(t *testing.T) {
	taskRoot := t.TempDir()
	worktree := filepath.Join(taskRoot, "repo")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	writeWorktreeGitFile(t, worktree)

	handle, err := OpenDirectoryNoFollow(taskRoot, worktree)
	if err != nil {
		t.Fatalf("OpenDirectoryNoFollow: %v", err)
	}
	if !handle.IsValidWorktree() {
		t.Fatal("IsValidWorktree returned false")
	}

	if err := handle.RemoveDirectory(context.Background()); err != nil {
		t.Fatalf("RemoveDirectory through a read-only validation handle: %v", err)
	}
	_ = handle.Close()

	if _, err := os.Lstat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree still present after RemoveDirectory: %v", err)
	}
}

func TestRemoveDirectoryFailsCleanlyWithShareIncompatibleHolder(t *testing.T) {
	taskRoot := t.TempDir()
	worktree := filepath.Join(taskRoot, "repo")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	sentinel := filepath.Join(worktree, "sentinel.txt")
	const sentinelContent = "keep this file"
	if err := os.WriteFile(sentinel, []byte(sentinelContent), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	handle, err := OpenDirectoryNoFollow(taskRoot, worktree)
	if err != nil {
		t.Fatalf("OpenDirectoryNoFollow: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	openShareIncompatibleHolder(t, worktree)

	err = handle.RemoveDirectory(context.Background())
	if err == nil {
		t.Fatal("RemoveDirectory succeeded despite a DELETE-incompatible holder")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("RemoveDirectory error = %v, want sharing violation", err)
	}
	if _, statErr := os.Lstat(worktree); statErr != nil {
		t.Fatalf("worktree removed despite the holder: %v", statErr)
	}
	content, readErr := os.ReadFile(sentinel)
	if readErr != nil {
		t.Fatalf("sentinel removed despite the holder: %v", readErr)
	}
	if string(content) != sentinelContent {
		t.Fatalf("sentinel content = %q, want %q", content, sentinelContent)
	}
}

func TestReadWriteFileThroughReadOnlyDirectoryHandle(t *testing.T) {
	taskRoot := t.TempDir()
	worktree := filepath.Join(taskRoot, "repo")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	handle, err := OpenDirectoryNoFollow(taskRoot, worktree)
	if err != nil {
		t.Fatalf("OpenDirectoryNoFollow: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	// The directory handle itself is read-only; child reads and writes open the
	// entry with their own access and must not depend on it.
	if err := handle.WriteFile("marker.json", []byte("payload"), 0o600); err != nil {
		t.Fatalf("WriteFile through a read-only directory handle: %v", err)
	}
	got, err := handle.ReadFile("marker.json")
	if err != nil {
		t.Fatalf("ReadFile through a read-only directory handle: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("ReadFile content = %q, want %q", got, "payload")
	}
}
