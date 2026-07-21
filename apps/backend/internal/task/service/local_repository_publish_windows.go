//go:build windows

package service

import (
	"fmt"
	"io/fs"
	"os"
)

func localRepositoryDirectoryOwnedByProcess(fs.FileInfo) bool {
	return true
}

func localRepositoryDirectoryOwnerTrusted(fs.FileInfo) bool {
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
	return os.Rename(stagingPath, targetPath)
}
