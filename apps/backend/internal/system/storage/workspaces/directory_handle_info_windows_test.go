//go:build windows

package workspaces

import "testing"

func TestPinnedDirectoryInfoRejectsTypedNilWindowsHandle(t *testing.T) {
	var handle DirectoryHandle = (*windowsDirectoryHandle)(nil)
	if _, err := PinnedDirectoryInfo(handle); err == nil {
		t.Fatal("PinnedDirectoryInfo accepted a typed-nil Windows directory handle")
	}
}
