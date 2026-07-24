//go:build linux

package service

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func localRepositoryDirectoryOwnedByProcess(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func localRepositoryDirectoryOwnerTrusted(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && (stat.Uid == 0 || stat.Uid == uint32(os.Geteuid()))
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
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("wrap directory descriptor")
	}
	return file, nil
}

func publishLocalRepository(stagingPath, targetPath string) error {
	return unix.Renameat2(unix.AT_FDCWD, stagingPath, unix.AT_FDCWD, targetPath, unix.RENAME_NOREPLACE)
}
