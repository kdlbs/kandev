//go:build !windows

package logbundle

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openBackendCandidate(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
