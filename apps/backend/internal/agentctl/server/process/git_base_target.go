package process

import (
	"context"
	"fmt"
	"strings"
)

// prepareBaseBranchTarget chooses the ref used by Merge and Rebase. An
// origin remote keeps the existing fetch-first behavior; without origin, the
// operation must use a verified local branch instead.
func (g *GitOperator) prepareBaseBranchTarget(
	ctx context.Context,
	baseBranch string,
) (output string, target string, err error) {
	remotesOutput, err := g.runGitCommand(ctx, "remote")
	if err != nil {
		return remotesOutput, "", fmt.Errorf("failed to list git remotes: %w", err)
	}

	if hasGitRemote(remotesOutput, "origin") {
		fetchOutput, fetchErr := g.runGitCommand(ctx, "fetch", "origin", baseBranch)
		if fetchErr != nil {
			return fetchOutput, "", fmt.Errorf("failed to fetch base branch: %w", fetchErr)
		}
		return fetchOutput, "origin/" + baseBranch, nil
	}

	localTarget := "refs/heads/" + baseBranch
	verifyOutput, verifyErr := g.runGitCommand(ctx, "rev-parse", "--verify", localTarget)
	if verifyErr != nil {
		return verifyOutput, "", fmt.Errorf("base branch %q does not exist locally: %w", baseBranch, verifyErr)
	}
	return "", localTarget, nil
}

func hasGitRemote(remotesOutput, wanted string) bool {
	for _, remote := range strings.Split(remotesOutput, "\n") {
		if strings.TrimSpace(remote) == wanted {
			return true
		}
	}
	return false
}
