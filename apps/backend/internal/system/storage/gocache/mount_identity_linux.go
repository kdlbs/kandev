//go:build linux

package gocache

import (
	"fmt"

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
