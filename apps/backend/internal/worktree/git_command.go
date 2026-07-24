package worktree

import (
	"context"
	"os/exec"
)

// newGitCommand enables Git's long-path handling for this process only. Git
// accepts the setting on every supported platform, so using the same command
// shape everywhere keeps all package-owned Git invocations on one safe path
// without mutating repository, global, or system configuration.
func newGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	commandArgs := make([]string, 0, len(args)+2)
	commandArgs = append(commandArgs, "-c", "core.longpaths=true")
	commandArgs = append(commandArgs, args...)
	return exec.CommandContext(ctx, "git", commandArgs...)
}
