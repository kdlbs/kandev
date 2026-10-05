//go:build unix

package workspaces

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func pinnedDirectoryInfo(handle DirectoryHandle) (os.FileInfo, error) {
	directory, ok := handle.(*unixDirectoryHandle)
	if !ok || directory == nil || directory.targetFD < 0 {
		return nil, errors.New("directory handle does not expose pinned Unix metadata")
	}
	fd, err := unix.Dup(directory.targetFD)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "pinned-workspace-directory")
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create metadata reader for pinned directory")
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil {
		return nil, statErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return info, nil
}
