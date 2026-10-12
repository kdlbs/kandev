//go:build !windows

package lifecycle

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openPassthroughMCPFileLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("create passthrough MCP lock handle for %q", path)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("passthrough MCP lock path %q is not a regular file", path)
	}
	return file, nil
}

func tryLockPassthroughMCPFile(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == unix.EWOULDBLOCK || err == unix.EAGAIN {
		return false, nil
	}
	return err == nil, err
}

func unlockPassthroughMCPFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
