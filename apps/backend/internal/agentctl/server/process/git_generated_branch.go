package process

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"go.uber.org/zap"
)

// RenameGeneratedBranch renames a Kandev-generated branch after verifying that
// the checkout still holds the expected symbolic ref. The check runs under the
// same checkout lock as turn capture, so a captured interval can transfer its
// live branch binding only across this verified operation.
func (g *GitOperator) RenameGeneratedBranch(
	ctx context.Context,
	expectedBranch, newName string,
) (*GitOperationResult, error) {
	if !securityutil.IsValidBranchName(expectedBranch) || !securityutil.IsValidBranchName(newName) {
		return nil, ErrInvalidBranchName
	}
	if !g.tryLock("rename_generated_branch") {
		return nil, ErrOperationInProgress
	}
	defer g.unlock()

	result := &GitOperationResult{Operation: "rename_branch"}
	gitDir, _, err := g.turnCheckpointRepository(ctx)
	if err != nil {
		return nil, err
	}
	unlock, err := acquireTurnCheckpointLock(ctx, gitDir)
	if err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	defer unlock()

	expectedRef := "refs/heads/" + expectedBranch
	oldCommit, currentRef, _, err := g.turnCheckpointHeadState(ctx)
	if err != nil {
		return nil, err
	}
	if currentRef != expectedRef {
		result.Error = fmt.Sprintf(
			"expected generated branch %q but HEAD is %q", expectedBranch, displayTurnCheckpointRef(currentRef))
		return result, nil
	}

	output, err := g.turnCheckpointOutput(ctx, "", "branch", "-m", expectedBranch, newName)
	result.Output = string(output)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	if err := g.verifyGeneratedBranchRename(ctx, expectedRef, oldCommit, newName); err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.Success = true
	g.logger.Info("generated branch renamed",
		zap.String("from", expectedBranch),
		zap.String("to", newName))
	if g.workspaceTracker != nil {
		g.workspaceTracker.RefreshGitStatus(ctx)
	}
	return result, nil
}

// verifyGeneratedBranchRename requires the expected new ref and unchanged real
// HEAD before the rename may transfer branch-binding continuity.
func (g *GitOperator) verifyGeneratedBranchRename(
	ctx context.Context,
	expectedOldRef, oldCommit, newName string,
) error {
	newCommit, newRef, _, err := g.turnCheckpointHeadState(ctx)
	if err != nil {
		g.resetTurnCheckpointBinding("")
		return err
	}
	expectedNewRef := "refs/heads/" + newName
	if newRef != expectedNewRef || newCommit != oldCommit {
		g.resetTurnCheckpointBinding(newRef)
		return fmt.Errorf("generated branch rename did not produce the expected ref and HEAD")
	}
	g.transferTurnCheckpointBinding(expectedOldRef, newRef)
	return nil
}

func displayTurnCheckpointRef(ref string) string {
	if short, found := strings.CutPrefix(ref, "refs/heads/"); found {
		return short
	}
	return ref
}
