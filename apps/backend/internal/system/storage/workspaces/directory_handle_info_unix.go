//go:build unix

package workspaces

import (
	"errors"
	"fmt"
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

// DirectoryIdentity returns the pinned filesystem identity for durable cleanup.
func DirectoryIdentity(handle DirectoryHandle) (string, error) {
	directory, ok := handle.(*unixDirectoryHandle)
	if !ok || directory == nil || directory.targetFD < 0 {
		return "", errors.New("directory handle does not expose pinned Unix identity")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(directory.targetFD, &stat); err != nil {
		return "", err
	}
	return fmt.Sprintf("unix:%d:%d", stat.Dev, stat.Ino), nil
}
