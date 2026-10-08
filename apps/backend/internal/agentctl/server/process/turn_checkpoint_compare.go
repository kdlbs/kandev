package process

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/turnchanges"
)

func (g *GitOperator) CompareTurnCheckpoints(
	ctx context.Context,
	request turnchanges.CompareRequest,
) (turnchanges.CheckpointComparison, error) {
	if !validTurnChangeIdentity(request.ChangeSetID) || !validTurnChangeIdentity(request.CheckoutID) {
		return turnchanges.CheckpointComparison{}, turnCheckpointFailure(turnchanges.ReasonUnsafeGitState, errors.New("invalid change-set or checkout identity"))
	}
	operationCtx, cancel := context.WithTimeout(ctx, turnCheckpointEndTimeout)
	defer cancel()
	gitDir, hashAlgorithm, err := g.turnCheckpointRepository(operationCtx)
	if err != nil {
		return turnchanges.CheckpointComparison{}, err
	}
	unlock, err := acquireTurnCheckpointLock(operationCtx, gitDir)
	if err != nil {
		return turnchanges.CheckpointComparison{}, turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	defer unlock()
	return g.compareTurnCheckpointLocked(operationCtx, request, hashAlgorithm)
}

func (g *GitOperator) compareTurnCheckpointLocked(
	ctx context.Context,
	request turnchanges.CompareRequest,
	hashAlgorithm string,
) (turnchanges.CheckpointComparison, error) {
	start, end, err := g.turnCheckpointPair(ctx, request, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointComparison{}, err
	}
	files, err := g.compareTurnCheckpointTrees(ctx, start.TreeOID, end.TreeOID)
	if err != nil {
		return turnchanges.CheckpointComparison{}, err
	}
	return summarizeTurnCheckpointComparison(request, files), nil
}

func (g *GitOperator) turnCheckpointPair(
	ctx context.Context,
	request turnchanges.CompareRequest,
	hashAlgorithm string,
) (turnchanges.CheckpointResult, turnchanges.CheckpointResult, error) {
	if request.HashAlgorithm != hashAlgorithm ||
		!validCheckpointOID(request.StartCommitOID, hashAlgorithm) || !validCheckpointOID(request.StartTreeOID, hashAlgorithm) ||
		!validCheckpointOID(request.EndCommitOID, hashAlgorithm) || !validCheckpointOID(request.EndTreeOID, hashAlgorithm) {
		return turnchanges.CheckpointResult{}, turnchanges.CheckpointResult{}, turnCheckpointFailure(
			turnchanges.ReasonUnsafeGitState, errors.New("accepted checkpoint endpoint pair is invalid"),
		)
	}
	start, err := g.checkpointResultForAcceptedOID(ctx, request.ChangeSetID, request.CheckoutID,
		turnchanges.CheckpointStart, request.StartCommitOID, request.StartTreeOID, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointResult{}, turnchanges.CheckpointResult{}, checkpointEndpointFailure("start", err, true)
	}
	end, err := g.checkpointResultForAcceptedOID(ctx, request.ChangeSetID, request.CheckoutID,
		turnchanges.CheckpointEnd, request.EndCommitOID, request.EndTreeOID, hashAlgorithm)
	if err != nil {
		return turnchanges.CheckpointResult{}, turnchanges.CheckpointResult{}, checkpointEndpointFailure("end", err, true)
	}
	return start, end, nil
}

func (g *GitOperator) checkpointResultForAcceptedOID(
	ctx context.Context,
	changeSetID, checkoutID string,
	boundary turnchanges.CheckpointBoundary,
	commitOID, treeOID, hashAlgorithm string,
) (turnchanges.CheckpointResult, error) {
	output, err := g.turnCheckpointOutput(ctx, "", "show", "-s", "--format=%T", commitOID)
	if err != nil {
		return turnchanges.CheckpointResult{}, turnCheckpointFailure(turnchanges.ReasonContentUnavailable, err)
	}
	if actualTreeOID := strings.TrimSpace(string(output)); actualTreeOID != treeOID {
		return turnchanges.CheckpointResult{}, turnCheckpointFailure(
			turnchanges.ReasonUnsafeGitState, errors.New("accepted checkpoint commit and tree do not match"),
		)
	}
	return turnchanges.CheckpointResult{
		ChangeSetID: changeSetID, CheckoutID: checkoutID, Boundary: boundary,
		CommitOID: commitOID, TreeOID: treeOID, HashAlgorithm: hashAlgorithm,
		ReachabilityRef: turnCheckpointRef(changeSetID, checkoutID, boundary),
	}, nil
}

func (g *GitOperator) compareTurnCheckpointTrees(ctx context.Context, startOID, endOID string) ([]turnchanges.CheckpointFile, error) {
	args := []string{"diff", "--raw", "-z", "--no-abbrev", "-M", "-C", "--find-copies-harder", "--no-ext-diff", "--no-textconv", startOID, endOID}
	rawOutput, err := g.turnCheckpointOutput(ctx, "", args...)
	if err != nil {
		return nil, checkpointCompareFailure(err)
	}
	numstatArgs := []string{"diff", "--numstat", "-z", "-M", "-C", "--find-copies-harder", "--no-ext-diff", "--no-textconv", startOID, endOID}
	numstatOutput, err := g.turnCheckpointOutput(ctx, "", numstatArgs...)
	if err != nil {
		return nil, checkpointCompareFailure(err)
	}
	rawChanges, err := turnchanges.ParseRawDiff(rawOutput)
	if err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	numstatChanges, err := turnchanges.ParseNumstat(numstatOutput)
	if err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	if len(rawChanges) > turnCheckpointEntryLimit || len(numstatChanges) > turnCheckpointEntryLimit {
		return nil, turnCheckpointFailure(turnchanges.ReasonEntryLimit, ErrTurnCheckpointOutputLimit)
	}
	files, err := turnchanges.JoinGitDiff(rawChanges, numstatChanges)
	if err != nil {
		return nil, turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
	}
	return files, nil
}

func checkpointEndpointFailure(boundary string, err error, found bool) error {
	if err != nil {
		return err
	}
	if !found {
		return turnCheckpointFailure(turnchanges.ReasonContentUnavailable, errors.New(boundary+" checkpoint ref is not available"))
	}
	return turnCheckpointFailure(turnchanges.ReasonContentUnavailable, errors.New(boundary+" checkpoint is unavailable"))
}

func checkpointCompareFailure(err error) error {
	if errors.Is(err, ErrTurnCheckpointOutputLimit) {
		return turnCheckpointFailure(turnchanges.ReasonSizeLimit, err)
	}
	return turnCheckpointFailure(turnchanges.ReasonComparisonFailed, err)
}

func summarizeTurnCheckpointComparison(
	request turnchanges.CompareRequest,
	files []turnchanges.CheckpointFile,
) turnchanges.CheckpointComparison {
	added, deleted := int64(0), int64(0)
	comparison := turnchanges.CheckpointComparison{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID,
		StartTreeOID: request.StartTreeOID, EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID,
		Files: files, FileCount: int64(len(files)), Complete: true,
	}
	for index := range comparison.Files {
		file := &comparison.Files[index]
		if !utf8.Valid(file.PathBytes) || len(file.OldPathBytes) > 0 && !utf8.Valid(file.OldPathBytes) {
			file.Reason = turnchanges.ReasonInvalidPathEncoding
			comparison.Complete = false
			comparison.Reason = turnchanges.ReasonInvalidPathEncoding
			comparison.UnknownCount++
		}
		if file.Binary {
			comparison.BinaryCount++
			continue
		}
		added += *file.Added
		deleted += *file.Deleted
	}
	comparison.AddedLines = &added
	comparison.DeletedLines = &deleted
	return comparison
}
