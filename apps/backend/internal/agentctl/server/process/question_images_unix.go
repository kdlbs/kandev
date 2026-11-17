//go:build !windows

package process

import (
	"os"
	"syscall"
)

func openQuestionImageFile(root *os.Root, path string) (*os.File, error) {
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOCTTY, 0)
}
