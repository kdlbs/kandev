//go:build !windows

package acp

import (
	"os"

	"golang.org/x/sys/unix"
)

func openClaudeMonitorContainedFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
}
