//go:build linux

package gocache

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func cacheMountIdentity(path string) (string, error) {
	var info unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &info); err != nil {
		return "", fmt.Errorf("inspect Go-cache mount for %q: %w", path, err)
	}
	if info.Mask&unix.STATX_MNT_ID == 0 {
		return "", fmt.Errorf("go-cache mount identity is unavailable for %q", path)
	}
	return fmt.Sprintf("mount:%d", info.Mnt_id), nil
}

func cacheMountIdentityFromFile(file *os.File) (string, error) {
	var info unix.Statx_t
	if err := unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &info); err != nil {
		return "", fmt.Errorf("inspect opened Go-cache mount: %w", err)
	}
	if info.Mask&unix.STATX_MNT_ID == 0 {
		return "", errors.New("go-cache mount identity is unavailable for opened path")
	}
	return fmt.Sprintf("mount:%d", info.Mnt_id), nil
}
