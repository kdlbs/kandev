//go:build windows

package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func localRepositoryDirectoryOwnedByProcess(fs.FileInfo) bool {
	return true
}

func localRepositoryDirectoryOwnerTrusted(fs.FileInfo) bool {
	return true
}

func localRepositoryParentWritable(info fs.FileInfo) bool {
	// Windows reports directory permissions as 0777; MkdirTemp performs the
	// authoritative write check when the staging directory is created.
	return info.Mode().Perm()&0o222 != 0
}

func localRepositoryParentSharedWritable(fs.FileInfo) bool {
	return false
}

func localRepositoryStagingPermissionsPrivate(fs.FileInfo) bool {
	return true
}

func openLocalRepositoryDirectory(path string) (*os.File, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.IsDir() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("path is not a directory")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fileInfo, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, fileInfo) {
		_ = file.Close()
		return nil, fmt.Errorf("directory changed while opening")
	}
	return file, nil
}

func publishLocalRepository(stagingPath, targetPath string) error {
	from, err := windows.UTF16PtrFromString(stagingPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, 0); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) ||
			errors.Is(err, windows.ERROR_FILE_EXISTS) || localRepositoryTargetExists(targetPath) {
			return fs.ErrExist
		}
		return err
	}
	return nil
}

func localRepositoryTargetExists(targetPath string) bool {
	_, err := os.Lstat(targetPath)
	return err == nil
}
