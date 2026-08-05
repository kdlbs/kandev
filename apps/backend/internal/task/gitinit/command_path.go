//go:build !linux && !darwin

package gitinit

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/kandev/kandev/internal/common/subproc"
)

// CommandContext creates a Git initialization command for platforms without inherited fd support.
func CommandContext(ctx context.Context, targetPath string, _ *os.File) (*exec.Cmd, error) {
	command := subproc.NewGitCommand(ctx, "init", "--initial-branch=main")
	command.Dir = targetPath
	return command, nil
}

// CommitCommandContext creates a Git commit command for targetPath.
func CommitCommandContext(ctx context.Context, targetPath string, _ *os.File, args ...string) (*exec.Cmd, error) {
	command := subproc.NewGitCommand(ctx, args...)
	command.Dir = targetPath
	return command, nil
}

func runInheritedDirectory(string) int {
	fmt.Fprintln(os.Stderr, "git init helper is unsupported on this platform")
	return 2
}
