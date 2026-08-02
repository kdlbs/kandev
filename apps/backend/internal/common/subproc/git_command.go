package subproc

import (
	"context"
	"os/exec"
)

// GitExecutablePath resolves the Git binary for helper processes that need to
// preserve a file-descriptor-based working directory. Keeping the lookup in
// the Git seam lets the repository audit cover both command construction and
// executable discovery.
func GitExecutablePath() (string, error) { return exec.LookPath("git") }

// NewGitCommand is the only production Git command-construction seam. Callers
// must pass the returned command to a classified RunGit* helper (or hold a
// classified slot around a streaming Start/Wait lifecycle).
func NewGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", args...)
}
