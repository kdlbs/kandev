//go:build !windows

package agents

import (
	"os"

	"golang.org/x/sys/unix"
)

// openFingerprintDir opens path only if it is a directory. O_DIRECTORY rejects
// any other file type atomically and O_NONBLOCK keeps a FIFO from blocking
// the open; symlinks to directories are still followed.
func openFingerprintDir(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
