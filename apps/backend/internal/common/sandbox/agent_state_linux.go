//go:build linux

package sandbox

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockAgentState(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		_ = file.Close()
		return nil, ErrInvalidPolicy
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, ErrInvalidPolicy
	}
	return file, nil
}
