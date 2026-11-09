//go:build unix

package process

import (
	"os"
	"syscall"
)

func projectMCPReadFlags() int {
	return os.O_RDONLY | syscall.O_NONBLOCK
}
