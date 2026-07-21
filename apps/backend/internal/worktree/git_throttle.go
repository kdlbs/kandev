package worktree

import (
	"context"
	"os/exec"

	"github.com/kandev/kandev/internal/common/subproc"
)

// Why this throttle exists:
//
// Worktree creation, cleanup, and submodule init all shell out to git in
// quick succession (worktree add, branch -D, worktree prune, submodule
// update --init --recursive, ...). When multiple agents start at once the
// number of concurrent git execs can climb past 30, and on managed
// corporate macOS hosts (CrowdStrike Falcon + syspolicyd) every fork/exec
// is intercepted, serialized, and validated. The resulting backlog has
// been observed alongside the gh-poller storm — see internal/common/
// subproc for the full rationale. This file just provides short
// package-local wrappers so callers don't have to reach into subproc
// for every git exec.

// runGitCmd acquires a git slot, runs cmd, and releases. Use this
// anywhere we exec a git binary from the worktree package — calling
// cmd.Run() directly bypasses the throttle.
func runGitCmd(ctx context.Context, cmd *exec.Cmd) error {
	return subproc.RunGit(ctx, cmd)
}

// runGitCmdCombinedOutput is runGitCmd's CombinedOutput sibling.
func runGitCmdCombinedOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	return subproc.RunGitCombinedOutput(ctx, cmd)
}

// runGitCmdOutput is runGitCmd's Output sibling.
func runGitCmdOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	return subproc.RunGitOutput(ctx, cmd)
}

// setGitThrottleCapForTest swaps the git throttle pool to the given
// capacity and returns a restore closure. Test-only.
func setGitThrottleCapForTest(cap int) func() {
	return subproc.Git().SetCapForTest(cap)
}
