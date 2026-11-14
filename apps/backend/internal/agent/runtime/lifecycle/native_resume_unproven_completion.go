package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// nativeResumeUnprovenCompletionVerifier re-reads the historical completion
// predicate without changing SQL state. The caller already authenticated the
// journal owner.
type nativeResumeUnprovenCompletionVerifier interface {
	VerifyUnprovenNativeResumeCompletion(context.Context, string, string, models.NativeResumeSubmissionEvidence) (bool, error)
}

// peerMatchesUnprovenCompletion identifies the historical foreground-handoff
// mismatch: SQL completed with no terminal proof against an interrupted-unknown
// journal entry with no retained terminal. The repository verifier must still
// pass; this function only matches the identities.
func peerMatchesUnprovenCompletion(execution *AgentExecution, status *agentctl.DeliveryStatus, peer journal.Submission, row *models.AgentDeliverySubmission) bool {
	return row != nil && row.State == models.DeliverySubmissionCompleted &&
		row.Outcome == string(models.DeliverySubmissionCompleted) &&
		row.PayloadHash != "" && peer.Hash == row.PayloadHash &&
		row.TerminalStreamID == "" && row.TerminalSequence == 0 && row.TerminalTurnID == "" &&
		row.FullySettledAt == nil && row.PayloadPrunedAt == nil &&
		peer.State == journal.SubmissionInterruptedUnknown &&
		!peer.TerminalEventRetained && peer.TerminalSequence == 0 &&
		nativeTerminalOwnerMatches(execution, status, peer, row)
}

func verifyNativeResumeUnprovenCompletion(
	ctx context.Context,
	repository AgentDeliveryRepository,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	peer journal.Submission,
	row *models.AgentDeliverySubmission,
) (bool, error) {
	verifier, supported := repository.(nativeResumeUnprovenCompletionVerifier)
	if !supported || row == nil {
		return false, nil
	}
	verified, err := verifier.VerifyUnprovenNativeResumeCompletion(
		ctx, execution.SessionID, status.IncarnationID,
		models.NativeResumeSubmissionEvidence{
			ID: row.ID, HarnessGeneration: row.HarnessGeneration, PayloadHash: row.PayloadHash,
			State: row.State, Outcome: row.Outcome, StreamID: peer.StreamID,
		},
	)
	if err != nil {
		return false, fmt.Errorf("%w: verify unproven native completion: %w", ErrDurableAdoptionBlocked, err)
	}
	return verified, nil
}

// nativeResumeUnprovenBackendRows includes eligible completed SQL metadata in
// loaded inventory discovery without broadening ordinary live adoption.
func nativeResumeUnprovenBackendRows(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	peer []journal.Submission,
	active []*models.AgentDeliverySubmission,
	repository AgentDeliveryRepository,
) ([]*models.AgentDeliverySubmission, error) {
	seen := make(map[string]bool, len(active))
	for _, row := range active {
		seen[row.ID] = true
	}
	var rows []*models.AgentDeliverySubmission
	for _, submission := range peer {
		if seen[submission.ID] {
			continue
		}
		row, include, err := nativeResumeUnprovenDiscoveryRow(ctx, execution, status, submission, repository)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		rows = append(rows, row)
		seen[row.ID] = true
	}
	return rows, nil
}

func nativeResumeUnprovenDiscoveryRow(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	submission journal.Submission,
	repository AgentDeliveryRepository,
) (*models.AgentDeliverySubmission, bool, error) {
	if submission.State != journal.SubmissionInterruptedUnknown || submission.Retired {
		return nil, false, nil
	}
	row, err := nativeResumeSubmissionMetadata(ctx, repository, submission.ID)
	if errors.Is(err, repoerrors.ErrAgentDeliverySubmissionNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if row == nil || !peerMatchesUnprovenCompletion(execution, status, submission, row) {
		return nil, false, nil
	}
	cursor, err := nativeResumeRowCursor(ctx, repository, submission)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !nativeTerminalCursorMatches(cursor, submission, 0) || cursor.ReceivedSequence < cursor.RemoteHighWater {
		return nil, false, nil
	}
	verified, err := verifyNativeResumeUnprovenCompletion(ctx, repository, execution, status, submission, row)
	if err != nil {
		return nil, false, err
	}
	if !verified {
		return nil, false, nil
	}
	return row, true, nil
}
