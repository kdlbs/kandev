package process

import (
	"context"
	"os"
	"os/exec"
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
