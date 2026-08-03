//go:build linux || darwin

package gitinit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/subproc"
	"golang.org/x/sys/unix"
)

const inheritedDirectoryFD = 3

// init handles the explicitly marked helper subprocess before a test binary or
// another Kandev entry point can interpret the private helper argument.
func init() {
	if os.Getenv(helperEnvironmentVariable) != "1" {
		return
	}
	if code, handled := runHelper(os.Args[1:]); handled {
		os.Exit(code)
	}
}

// CommandContext creates a Git initialization command bound to targetDirectory.
func CommandContext(ctx context.Context, _ string, targetDirectory *os.File) (*exec.Cmd, error) {
	gitPath, err := subproc.GitExecutablePath()
	if err != nil {
		return nil, fmt.Errorf("find git: %w", err)
	}
	if !filepath.IsAbs(gitPath) {
		gitPath, err = filepath.Abs(gitPath)
		if err != nil {
			return nil, fmt.Errorf("resolve git executable: %w", err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve kandev executable: %w", err)
	}
	command := exec.CommandContext(ctx, executable, helperArgument, gitPath)
	command.ExtraFiles = []*os.File{targetDirectory}
	command.Env = withHelperEnvironment(os.Environ())
	return command, nil
}

func runInheritedDirectory(gitPath string) int {
	if err := unix.Fchdir(inheritedDirectoryFD); err != nil {
		fmt.Fprintf(os.Stderr, "git init helper: enter inherited directory: %v\n", err)
		return 1
	}
	if err := subproc.ExecGit(
		gitPath,
		[]string{"init", "--initial-branch=main"},
		withoutHelperEnvironment(os.Environ()),
	); err != nil {
		fmt.Fprintf(os.Stderr, "git init helper: execute git: %v\n", err)
		return 1
	}
	return 0
}
