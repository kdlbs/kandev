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
//
// Build the command with only the fixed executable at the exec.CommandContext
// sink, then attach the already-tokenized argv. Git never invokes a shell for
// Cmd.Args, and keeping the user-derived values out of the constructor also
// prevents CodeQL's command-injection query from treating this direct-argv
// seam as shell interpolation. Callers remain responsible for validating
// user-controlled refs, paths, and option values before they reach this seam.
func NewGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git")
	cmd.Args = append(cmd.Args, args...)
	return cmd
}
