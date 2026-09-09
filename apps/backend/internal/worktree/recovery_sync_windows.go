//go:build windows

package worktree

import (
	"fmt"
	"os"
)

func syncRecoveryFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open recovery state for sync: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync recovery state: %w", err)
	}
	return nil
}

func syncRecoveryDirectory(string) error {
	return nil
}
