//go:build windows

package workspaces

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func pinnedDirectoryInfo(handle DirectoryHandle) (os.FileInfo, error) {
	directory, ok := handle.(*windowsDirectoryHandle)
	if !ok || directory == nil || directory.targetHandle == 0 {
		return nil, errors.New("directory handle does not expose pinned Windows metadata")
	}
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(
		windows.CurrentProcess(), directory.targetHandle, windows.CurrentProcess(), &duplicate,
		0, false, windows.DUPLICATE_SAME_ACCESS,
	); err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "pinned-workspace-directory")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
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
	directory, ok := handle.(*windowsDirectoryHandle)
	if !ok || directory == nil || directory.targetHandle == 0 {
		return "", errors.New("directory handle does not expose pinned Windows identity")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(directory.targetHandle, &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("windows:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
