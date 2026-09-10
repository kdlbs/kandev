//go:build !windows

package worktree

import (
	"fmt"
	"os"
)

func syncRecoveryFile(file *os.File) error {
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync recovery state: %w", err)
	}
	return nil
}

func syncRecoveryDirectory(path string) error {
	return syncRecoveryPath(path, os.O_RDONLY)
}

func syncRecoveryPath(path string, flag int) error {
	file, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return fmt.Errorf("open recovery state for sync: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync recovery state: %w", err)
	}
	return nil
}
