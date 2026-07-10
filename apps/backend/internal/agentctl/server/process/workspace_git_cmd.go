package process

import (
	"context"
	"os"
	"os/exec"

	"github.com/kandev/kandev/internal/common/subproc"
)

// gitOptionalLocksOff is the env var git reads to skip "optional" locks, i.e.
// the index refresh lock that `git status` and friends take to update stat
// info. The workspace tracker polls git read-only, but without this flag even
// those reads can briefly take `.git/index.lock`, racing with concurrent user
// operations (stage, commit) that need it for writes — in tight-loop polling
// this manifests as sporadic "Unable to create '.../index.lock': File exists"
// failures from user commands.
//
// See: https://git-scm.com/docs/git#Documentation/git.txt-codeGITOPTIONALLOCKSltbooleangtcode
const gitOptionalLocksOff = "GIT_OPTIONAL_LOCKS=0"

// pollingGitCommand builds an exec.Cmd for a polling git invocation. It sets
// the workspace directory and disables optional locks so the background poll
// loop doesn't contend with user-initiated git operations.
func (wt *WorkspaceTracker) pollingGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = wt.workDir
	cmd.Env = append(os.Environ(), gitOptionalLocksOff)
	return cmd
}

// gitCmdContext builds a per-command timeout-bounded ctx and an exec.Cmd
// for a git invocation. When polling is true, the cmd is built via
// pollingGitCommand (which sets GIT_OPTIONAL_LOCKS=0) so the background
// poll loop doesn't contend with user-initiated git operations on the
// index lock. The returned cancel must be deferred by the caller to
// release the timer once the subprocess exits.
func (wt *WorkspaceTracker) gitCmdContext(ctx context.Context, polling bool, args ...string) (context.Context, context.CancelFunc, *exec.Cmd) {
	cctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	if polling {
		return cctx, cancel, wt.pollingGitCommand(cctx, args...)
	}
	cmd := exec.CommandContext(cctx, "git", args...)
	cmd.Dir = wt.workDir
	return cctx, cancel, cmd
}

// runGitOutput runs a git command with a per-command timeout and returns its
// stdout. The derived ctx ensures cancellation SIGKILLs the subprocess and
// releases its throttle slot even when the outer poll ctx is long-lived.
func (wt *WorkspaceTracker) runGitOutput(ctx context.Context, args ...string) ([]byte, error) {
	cctx, cancel, cmd := wt.gitCmdContext(ctx, false, args...)
	defer cancel()
	return subproc.RunGitOutput(cctx, cmd)
}

// runGit is runGitOutput's no-stdout sibling for verify-style probes where
// only the exit code matters.
func (wt *WorkspaceTracker) runGit(ctx context.Context, args ...string) error {
	cctx, cancel, cmd := wt.gitCmdContext(ctx, false, args...)
	defer cancel()
	return subproc.RunGit(cctx, cmd)
}

// runPollingGitOutput is runGitOutput's polling sibling — see
// gitCmdContext for the GIT_OPTIONAL_LOCKS rationale.
func (wt *WorkspaceTracker) runPollingGitOutput(ctx context.Context, args ...string) ([]byte, error) {
	cctx, cancel, cmd := wt.gitCmdContext(ctx, true, args...)
	defer cancel()
	return subproc.RunGitOutput(cctx, cmd)
}
