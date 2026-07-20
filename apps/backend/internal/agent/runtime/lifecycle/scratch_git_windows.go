//go:build windows

package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func openGitExcludeNoFollow(gitDir string) (*os.File, error) {
	path := filepath.Join(gitDir, "info", "exclude")
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// OPEN_REPARSE_POINT protects the final component. Intermediate junctions
	// rely on the earlier Lstat checks; atomic traversal would require NtCreateFile.
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.FILE_APPEND_DATA,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("git exclude file is a reparse point")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("create file from git exclude handle")
	}
	return file, nil
}
