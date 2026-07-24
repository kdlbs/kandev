//go:build !linux && !darwin && !windows

package service

import (
	"fmt"
	"io/fs"
	"os"
)

func localRepositoryDirectoryOwnedByProcess(fs.FileInfo) bool {
	return false
}

func localRepositoryDirectoryOwnerTrusted(fs.FileInfo) bool {
	return false
}

func localRepositoryParentWritable(info fs.FileInfo) bool {
	return info.Mode().Perm()&0o222 != 0
}

func localRepositoryParentSharedWritable(info fs.FileInfo) bool {
	return info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0
}

func localRepositoryStagingPermissionsPrivate(info fs.FileInfo) bool {
	return info.Mode().Perm() == 0o700
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

func publishLocalRepository(_, _ string) error {
	return fmt.Errorf("exclusive repository publication is unsupported on this platform")
}
