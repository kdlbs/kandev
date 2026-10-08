package process

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

// DeleteTurnCheckpoint removes only a Kandev-owned endpoint ref and verifies
// that it still points at the accepted commit before deletion.
func (g *GitOperator) DeleteTurnCheckpoint(ctx context.Context, request turnchanges.CheckpointDeleteRequest) error {
	if !validTurnChangeIdentity(request.ChangeSetID) || !validTurnChangeIdentity(request.CheckoutID) ||
		(request.Boundary != turnchanges.CheckpointStart && request.Boundary != turnchanges.CheckpointEnd) {
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("invalid checkpoint deletion identity"))
	}
	operationCtx, cancel := context.WithTimeout(ctx, turnCheckpointStartTimeout)
	defer cancel()
	gitDir, hashAlgorithm, err := g.turnCheckpointRepository(operationCtx)
	if err != nil {
		return err
	}
	if !validCheckpointOID(request.CommitOID, hashAlgorithm) {
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("invalid expected checkpoint commit"))
	}
	unlock, err := acquireTurnCheckpointLock(operationCtx, gitDir)
	if err != nil {
		return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	defer unlock()
	ref := turnCheckpointRef(request.ChangeSetID, request.CheckoutID, request.Boundary)
	output, err := g.turnCheckpointOutput(operationCtx, "", "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	currentOID := strings.TrimSpace(string(output))
	if currentOID == "" {
		return nil
	}
	if currentOID != request.CommitOID {
		return turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("owned checkpoint ref moved from its accepted commit"))
	}
	if _, err := g.turnCheckpointOutput(operationCtx, "", "update-ref", "-d", ref, request.CommitOID); err != nil {
		return turnCheckpointFailure(turnchanges.ReasonCaptureFailed, err)
	}
	return nil
}
