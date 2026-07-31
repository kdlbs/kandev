//go:build !linux && !darwin

package gitinit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CommandContext creates a Git initialization command for platforms without inherited fd support.
func CommandContext(ctx context.Context, targetPath string, _ *os.File) (*exec.Cmd, error) {
	command := exec.CommandContext(ctx, "git", "init", "--initial-branch=main")
	command.Dir = targetPath
	return command, nil
}

func runInheritedDirectory(string) int {
	fmt.Fprintln(os.Stderr, "git init helper is unsupported on this platform")
	return 2
}
