//go:build !windows

package acp

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openClaudeMonitorContainedFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
}

func openClaudeMonitorContainedDirectory(root *os.Root, name string) (*os.Root, error) {
	file, err := root.OpenFile(
		name,
		os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return os.OpenRoot(fmt.Sprintf("/dev/fd/%d", file.Fd()))
}
