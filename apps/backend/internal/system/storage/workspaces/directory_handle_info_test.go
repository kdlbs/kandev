//go:build unix

package workspaces

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPinnedDirectoryInfoReadsModeAndOwnerFromHandle(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	mode := os.FileMode(0o750) | os.ModeSetgid
	if err := os.Chmod(directory, mode); err != nil {
		t.Fatalf("set directory mode: %v", err)
	}
	originalInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("stat original directory after chmod: %v", err)
	}
	handle, err := OpenDirectoryNoFollow(parent, directory)
	if err != nil {
		t.Fatalf("OpenDirectoryNoFollow: %v", err)
	}
	t.Cleanup(func() {
		if err := handle.Close(); err != nil {
			t.Errorf("close directory handle: %v", err)
		}
	})

	oldPath := directory + ".renamed"
	if err := os.Rename(directory, oldPath); err != nil {
		t.Fatalf("rename opened directory: %v", err)
	}
	if err := os.Mkdir(directory, 0o711); err != nil {
		t.Fatalf("replace opened directory path: %v", err)
	}

	pinned, err := PinnedDirectoryInfo(handle)
	if err != nil {
		t.Fatalf("PinnedDirectoryInfo: %v", err)
	}
	pathInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("stat directory path: %v", err)
	}
	if !os.SameFile(originalInfo, pinned) {
		t.Fatalf("pinned info does not describe the opened directory: pinned=%v original=%v", pinned, originalInfo)
	}
	if os.SameFile(pathInfo, pinned) || pinned.Mode() == pathInfo.Mode() {
		t.Fatalf("pinned info followed the replacement path: pinned=%v path=%v", pinned, pathInfo)
	}
	pinnedOwner, pinnedOK := pinned.Sys().(*syscall.Stat_t)
	originalOwner, originalOK := originalInfo.Sys().(*syscall.Stat_t)
	if !pinnedOK || !originalOK || pinnedOwner.Uid != originalOwner.Uid || pinnedOwner.Gid != originalOwner.Gid {
		t.Fatalf("pinned owner = %#v, original owner = %#v", pinned.Sys(), originalInfo.Sys())
	}
}

func TestPinnedDirectoryInfoRejectsTypedNilUnixHandle(t *testing.T) {
	var handle DirectoryHandle = (*unixDirectoryHandle)(nil)
	if _, err := PinnedDirectoryInfo(handle); err == nil {
		t.Fatal("PinnedDirectoryInfo accepted a typed-nil Unix directory handle")
	}
}
