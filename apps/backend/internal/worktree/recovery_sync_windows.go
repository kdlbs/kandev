//go:build windows

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

func syncRecoveryDirectory(string) error {
	return nil
}
