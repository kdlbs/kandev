//go:build windows

package workspaces

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/kandev/kandev/internal/common/winpath"
)

// createWindowsJunction links a directory through an NTFS junction. Unlike a
// symbolic link, creating a junction requires no SeCreateSymbolicLinkPrivilege,
// so the reparse-point rejection path stays provable on an unprivileged host.
func createWindowsJunction(t *testing.T, link, target string) {
	t.Helper()
	command := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create junction %s -> %s: %v\n%s", link, target, err, output)
	}
	if _, err := os.Stat(link); err != nil {
		t.Fatalf("junction %s does not resolve to its target: %v", link, err)
	}
}

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

// A plain directory below the drive root must open through the volume-root
// traversal: the NT path starts with the object-manager drive-letter link,
// which a no-reparse open would reject with STATUS_REPARSE_POINT_ENCOUNTERED.
func TestOpenDirectoryNoFollowOpensPlainTempDirectory(t *testing.T) {
	taskRoot := t.TempDir()
	worktree := filepath.Join(taskRoot, "nested", "repo")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	writeWorktreeGitFile(t, worktree)

	handle, err := OpenDirectoryNoFollow(taskRoot, worktree)
	if err != nil {
		t.Fatalf("OpenDirectoryNoFollow plain directory: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	if !handle.IsValidWorktree() {
		t.Fatal("IsValidWorktree returned false for the opened worktree")
	}
	if err := handle.VerifyPath(worktree); err != nil {
		t.Fatalf("VerifyPath through the opened handle: %v", err)
	}
	entries, err := handle.ReadDir()
	if err != nil || len(entries) != 1 || entries[0].Name() != ".git" {
		t.Fatalf("ReadDir = %v, %v; want the worktree's .git entry", entries, err)
	}
}

// The volume root is the only open without OBJ_DONT_REPARSE. A link below it
// must still be rejected before it can redirect the walk.
func TestOpenDirectoryNoFollowRejectsReparsePointComponent(t *testing.T) {
	taskRoot := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(taskRoot, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "repo"), 0o755); err != nil {
		t.Fatalf("mkdir outside repo: %v", err)
	}

	if _, err := OpenDirectoryNoFollow(taskRoot, filepath.Join(link, "repo")); err == nil {
		t.Fatal("OpenDirectoryNoFollow followed a symlinked path component")
	}
}

// Junctions are reparse points that any user can create, so they exercise the
// same rejection path as symlinks without SeCreateSymbolicLinkPrivilege. This
// test must never skip: the security guarantee of the volume-root traversal is
// only proven when a user-controlled reparse point below the root is refused.
func TestOpenDirectoryNoFollowRejectsJunctions(t *testing.T) {
	t.Run("rejects junction path component", func(t *testing.T) {
		taskRoot := t.TempDir()
		outside := t.TempDir()
		repository := filepath.Join(outside, "repo")
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatalf("mkdir outside repository: %v", err)
		}
		writeWorktreeGitFile(t, repository)
		junction := filepath.Join(taskRoot, "linked")
		createWindowsJunction(t, junction, outside)

		// A plain sibling under the same root must still open, so the failure
		// below is attributable to the junction and not to the root traversal.
		plain := filepath.Join(taskRoot, "plain", "repo")
		if err := os.MkdirAll(plain, 0o755); err != nil {
			t.Fatalf("mkdir plain sibling: %v", err)
		}
		writeWorktreeGitFile(t, plain)
		plainHandle, err := OpenDirectoryNoFollow(taskRoot, plain)
		if err != nil {
			t.Fatalf("OpenDirectoryNoFollow plain sibling: %v", err)
		}
		_ = plainHandle.Close()

		if _, err := OpenDirectoryNoFollow(taskRoot, filepath.Join(junction, "repo")); err == nil {
			t.Fatal("OpenDirectoryNoFollow followed a junction path component")
		}
	})

	t.Run("rejects junction target", func(t *testing.T) {
		taskRoot := t.TempDir()
		outside := t.TempDir()
		writeWorktreeGitFile(t, outside)
		junction := filepath.Join(taskRoot, "linked")
		createWindowsJunction(t, junction, outside)

		if _, err := OpenDirectoryNoFollow(taskRoot, junction); err == nil {
			t.Fatal("OpenDirectoryNoFollow accepted a junction as the worktree target")
		}
	})

	t.Run("opens plain directory", func(t *testing.T) {
		taskRoot := t.TempDir()
		worktree := filepath.Join(taskRoot, "plain", "repo")
		if err := os.MkdirAll(worktree, 0o755); err != nil {
			t.Fatalf("mkdir plain worktree: %v", err)
		}
		writeWorktreeGitFile(t, worktree)

		handle, err := OpenDirectoryNoFollow(taskRoot, worktree)
		if err != nil {
			t.Fatalf("OpenDirectoryNoFollow plain directory: %v", err)
		}
		t.Cleanup(func() { _ = handle.Close() })
		if !handle.IsValidWorktree() {
			t.Fatal("IsValidWorktree returned false for the opened plain worktree")
		}
	})
}

// assertNoFollowRefusesRoot proves that a root reached through a junction is
// refused by every no-follow operation and that neither the junction target
// nor anything below it is created or removed.
func assertNoFollowRefusesRoot(t *testing.T, root, target, externalRoot string) {
	t.Helper()

	if handle, err := OpenDirectoryNoFollow(root, target); err == nil {
		_ = handle.Close()
		t.Fatal("OpenDirectoryNoFollow followed a junction in the root path")
	}

	created := filepath.Join(target, "created")
	if handle, err := CreateDirectoryNoFollow(root, created, 0o755); err == nil {
		_ = handle.Close()
		t.Fatal("CreateDirectoryNoFollow followed a junction in the root path")
	}
	if _, err := os.Lstat(filepath.Join(externalRoot, "created")); !os.IsNotExist(err) {
		t.Fatalf("CreateDirectoryNoFollow created %s in the junction target: %v",
			filepath.Join(externalRoot, "created"), err)
	}

	removable := filepath.Join(target, "node_modules")
	sentinel := filepath.Join(removable, "sentinel.txt")
	if err := RemoveDirectoryNoFollow(context.Background(), root, removable); err == nil {
		t.Fatal("RemoveDirectoryNoFollow followed a junction in the root path")
	}
	if _, err := os.Lstat(sentinel); err != nil {
		t.Fatalf("RemoveDirectoryNoFollow removed %s in the junction target: %v", sentinel, err)
	}
}

// The volume-root open is the only one that resolves a namespace link. A
// junction that is an ancestor of the root, or that is the root itself, must
// still be refused by the component walk before any component is used.
func TestNoFollowRefusesJunctionInRootPath(t *testing.T) {
	t.Run("junction ancestor of the root", func(t *testing.T) {
		outside := t.TempDir()
		externalRoot := filepath.Join(outside, "sub")
		repository := filepath.Join(externalRoot, "repo")
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatalf("mkdir external repository: %v", err)
		}
		writeWorktreeGitFile(t, repository)
		if err := os.MkdirAll(filepath.Join(repository, "node_modules"), 0o755); err != nil {
			t.Fatalf("mkdir external node_modules: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repository, "node_modules", "sentinel.txt"), []byte("keep"), 0o600); err != nil {
			t.Fatalf("write external sentinel: %v", err)
		}

		junction := filepath.Join(t.TempDir(), "linked")
		createWindowsJunction(t, junction, outside)

		root := filepath.Join(junction, "sub")
		assertNoFollowRefusesRoot(t, root, filepath.Join(root, "repo"), repository)
	})

	t.Run("root is the junction", func(t *testing.T) {
		outside := t.TempDir()
		repository := filepath.Join(outside, "repo")
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatalf("mkdir external repository: %v", err)
		}
		writeWorktreeGitFile(t, repository)
		if err := os.MkdirAll(filepath.Join(repository, "node_modules"), 0o755); err != nil {
			t.Fatalf("mkdir external node_modules: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repository, "node_modules", "sentinel.txt"), []byte("keep"), 0o600); err != nil {
			t.Fatalf("write external sentinel: %v", err)
		}

		root := filepath.Join(t.TempDir(), "linked")
		createWindowsJunction(t, root, outside)
		assertNoFollowRefusesRoot(t, root, filepath.Join(root, "repo"), repository)
	})
}

func TestCreateDirectoryNoFollowCreatesNestedDirectory(t *testing.T) {
	taskRoot := t.TempDir()
	target := filepath.Join(taskRoot, "task-one", "repo")

	handle, err := CreateDirectoryNoFollow(taskRoot, target, 0o755)
	if err != nil {
		t.Fatalf("CreateDirectoryNoFollow: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("created target stat = %v, %v; want a directory", info, err)
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
	deleteHandle, err := winpath.OpenDirectoryPath(worktree, windowsDependencyDeleteAccess)
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
