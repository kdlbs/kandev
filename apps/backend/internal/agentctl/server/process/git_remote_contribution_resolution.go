package process

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/securityutil"
	"go.uber.org/zap"
)

const (
	contributionLeaseFlagPrefix    = "--force-with-lease=refs/heads/"
	contributionErrorLeaseMismatch = "lease_mismatch"
	contributionErrorDirtyWorktree = "dirty_worktree"
	contributionErrorRemoteFailure = "remote_unavailable"
)

func validateContributionLeaseFlag(arg string) error {
	lease := strings.TrimPrefix(arg, contributionLeaseFlagPrefix)
	branch, expectedHead, ok := strings.Cut(lease, ":")
	if !ok || strings.Contains(expectedHead, ":") || !securityutil.IsValidBranchName(branch) ||
		(expectedHead != "" && !isFullCommitSHA(expectedHead)) {
		return fmt.Errorf("potentially unsafe flag: %s", arg)
	}
	return nil
}

func isFullCommitSHA(sha string) bool {
	if len(sha) != 40 && len(sha) != 64 {
		return false
	}
	for _, r := range sha {
		if !isHexCharacter(r) {
			return false
		}
	}
	return true
}

func isHexCharacter(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func (g *GitOperator) validateContributionResolution(ctx context.Context, expectedRemoteHead string) error {
	if g.remoteContributionErr != nil {
		return g.remoteContributionErr
	}
	if g.remoteContribution == nil {
		return errors.New("remote contribution binding is required")
	}
	if !isFullCommitSHA(expectedRemoteHead) {
		return errors.New("expected remote head must be a full commit ID")
	}
	return g.validateContributionRemote(ctx)
}

// ReplaceRemoteContribution publishes the local task HEAD to the bound
// contribution branch only when the provider head still matches the expected
// full commit ID.
func (g *GitOperator) ReplaceRemoteContribution(ctx context.Context, expectedRemoteHead string) (*GitOperationResult, error) {
	if !g.tryLock("replace_remote_contribution") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{Operation: "replace_remote_contribution"}
	if err := g.validateContributionResolution(ctx, expectedRemoteHead); err != nil {
		result.Error = err.Error()
		return result, nil
	}

	binding := g.remoteContribution
	lease := contributionLeaseFlagPrefix + binding.HeadBranch + ":" + expectedRemoteHead
	refspec := "HEAD:refs/heads/" + binding.HeadBranch
	output, err := g.runGitCommand(ctx, "push", lease, binding.ContributionRemoteName(), refspec)
	result.Output = output
	if err != nil {
		result.Error = err.Error()
		result.ErrorCode = contributionErrorCode(result.Error)
		return result, nil
	}

	result.Success = true
	g.logger.Info("remote contribution replaced",
		zap.String("branch", binding.HeadBranch),
		zap.String("remote", binding.ContributionRemoteName()))
	return result, nil
}

// UseRemoteContribution adopts the confirmed provider head after preserving
// the current task HEAD on a local recovery branch.
func (g *GitOperator) UseRemoteContribution(ctx context.Context, expectedRemoteHead string) (*GitOperationResult, error) {
	if !g.tryLock("use_remote_contribution") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{Operation: "use_remote_contribution"}
	if err := g.validateContributionResolution(ctx, expectedRemoteHead); err != nil {
		result.Error = err.Error()
		return result, nil
	}

	if g.rejectDirtyContributionWorktree(ctx, result) {
		return result, nil
	}

	binding := g.remoteContribution
	remote := binding.ContributionRemoteName()
	fetchOutput, err := g.runGitCommand(ctx, "fetch", remote, binding.HeadBranch)
	result.Output = fetchOutput
	if err != nil {
		setContributionError(result, contributionErrorRemoteFailure, fmt.Errorf("failed to fetch remote contribution: %w", err).Error())
		return result, nil
	}

	fetchedHead, err := g.runGitCommand(ctx, "rev-parse", remote+"/"+binding.HeadBranch)
	if err != nil {
		setContributionError(result, contributionErrorRemoteFailure, fmt.Errorf("failed to resolve fetched remote contribution: %w", err).Error())
		return result, nil
	}
	fetchedHead = strings.TrimSpace(fetchedHead)
	if fetchedHead != expectedRemoteHead {
		setContributionError(result, contributionErrorLeaseMismatch, fmt.Sprintf("remote contribution head changed before adoption: expected %s, fetched %s", expectedRemoteHead, fetchedHead))
		return result, nil
	}

	currentHead, err := g.runGitCommand(ctx, "rev-parse", "HEAD")
	if err != nil {
		result.Error = fmt.Errorf("failed to resolve current task HEAD: %w", err).Error()
		return result, nil
	}
	currentHead = strings.TrimSpace(currentHead)
	recoveryBranch, branchOutput, err := g.createRecoveryBranch(ctx, currentHead)
	if err != nil {
		result.Output = strings.TrimSpace(strings.Join([]string{result.Output, branchOutput}, "\n"))
		result.Error = err.Error()
		return result, nil
	}
	result.RecoveryBranch = recoveryBranch
	result.Output = strings.TrimSpace(strings.Join([]string{result.Output, branchOutput}, "\n"))

	// Fetching the provider ref and creating the recovery branch can give
	// another workspace writer time to modify the worktree. Check again
	// immediately before the destructive reset so those changes are preserved.
	if g.rejectDirtyContributionWorktree(ctx, result) {
		return result, nil
	}

	resetOutput, err := g.runGitCommand(ctx, "reset", "--hard", fetchedHead)
	result.Output = strings.TrimSpace(strings.Join([]string{result.Output, resetOutput}, "\n"))
	if err != nil {
		result.Error = fmt.Errorf("failed to use remote contribution: %w", err).Error()
		return result, nil
	}

	result.Success = true
	g.logger.Info("remote contribution adopted",
		zap.String("branch", binding.HeadBranch),
		zap.String("recovery_branch", recoveryBranch))
	return result, nil
}

func (g *GitOperator) rejectDirtyContributionWorktree(ctx context.Context, result *GitOperationResult) bool {
	dirty, err := g.hasUncommittedChanges(ctx)
	if err != nil {
		result.Error = err.Error()
		return true
	}
	if !dirty {
		return false
	}
	setContributionError(result, contributionErrorDirtyWorktree, "working tree must be clean before using remote contribution")
	return true
}

func setContributionError(result *GitOperationResult, code, message string) {
	result.Error = message
	result.ErrorCode = code
}

func contributionErrorCode(message string) string {
	normalized := strings.ToLower(message)
	if strings.Contains(normalized, "stale info") ||
		strings.Contains(normalized, "head changed") ||
		strings.Contains(normalized, "non-fast-forward") {
		return contributionErrorLeaseMismatch
	}
	if strings.Contains(normalized, "could not resolve host") ||
		strings.Contains(normalized, "unable to access") ||
		strings.Contains(normalized, "authentication failed") ||
		strings.Contains(normalized, "repository not found") {
		return contributionErrorRemoteFailure
	}
	return ""
}

func (g *GitOperator) createRecoveryBranch(ctx context.Context, currentHead string) (string, string, error) {
	if !isFullCommitSHA(currentHead) {
		return "", "", errors.New("current task HEAD is not a full commit ID")
	}

	baseName := fmt.Sprintf("kandev/recovery-%d-%s", time.Now().UTC().UnixNano(), currentHead[:12])
	for attempt := 0; attempt < 10; attempt++ {
		name := baseName
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d", baseName, attempt)
		}
		if _, err := g.runGitCommand(ctx, "rev-parse", "--verify", "refs/heads/"+name); err == nil {
			continue
		}
		output, err := g.runGitCommand(ctx, "branch", name, currentHead)
		if err != nil {
			return "", output, fmt.Errorf("failed to create recovery branch: %w", err)
		}
		return name, output, nil
	}
	return "", "", errors.New("failed to create a unique recovery branch")
}
