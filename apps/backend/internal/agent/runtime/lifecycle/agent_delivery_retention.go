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

var errDeliveryReplayReconciled = errors.New("expired agent delivery replay reconciled")

// ErrAgentDeliveryHistoryGap identifies journal output missing from SQL receipts.
var ErrAgentDeliveryHistoryGap = errors.New(models.AgentDeliveryHistoryGapFailureCode)

type agentDeliveryStatusReader interface {
	GetDeliveryStatus(context.Context, string) (*agentctl.DeliveryStatus, error)
}

func authenticatedDeliveryReplayCursor(
	ctx context.Context,
	execution *AgentExecution,
	repository AgentDeliveryRepository,
	status *agentctl.DeliveryStatus,
) (uint64, error) {
	if err := validateAuthenticatedDeliveryDescriptor(execution, status); err != nil {
		return 0, err
	}
	if err := validateRecoveredStream(status); err != nil {
		return 0, err
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, status.StreamID)
	if errors.Is(err, sql.ErrNoRows) {
		if status.Stream == nil || status.Stream.HighWater == 0 {
			return 0, nil
		}
		if status.Stream.FirstRetained > 1 && status.Stream.FirstRetained <= status.Stream.HighWater+1 {
			return 0, fmt.Errorf("%w: %s", ErrAgentDeliveryHistoryGap, models.AgentDeliveryHistoryGapDetails(status.StreamID, 1, int64(status.Stream.FirstRetained-1)))
		}
		if canReplayFreshBootstrapEvents(ctx, execution, repository, status) {
			return 0, nil
		}
		return 0, repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	if err != nil {
		return 0, err
	}
	if err := validateAuthenticatedSQLDeliveryCursor(cursor, status); err != nil {
		return 0, err
	}
	return projectedCursorCoveredByPeer(cursor, status)
}

func validateAuthenticatedDeliveryDescriptor(execution *AgentExecution, status *agentctl.DeliveryStatus) error {
	if execution == nil {
		return fmt.Errorf("authenticated delivery descriptor has no execution: %w", ErrDurableAdoptionIdentityMismatch)
	}
	expectedStreamID := execution.DeliveryStreamID
	if expectedStreamID == "" {
		expectedStreamID = execution.SessionID
	}
	if status == nil || !status.Durable || status.Version != journal.CurrentVersion ||
		status.SessionID != execution.SessionID || status.IncarnationID != execution.DeliveryIncarnationID ||
		status.HarnessGeneration != execution.DeliveryHarnessGeneration || status.StreamID != expectedStreamID {
		return fmt.Errorf("authenticated delivery descriptor mismatch: %w", ErrDurableAdoptionIdentityMismatch)
	}
	return nil
}

func validateAuthenticatedSQLDeliveryCursor(cursor *models.AgentDeliveryCursor, status *agentctl.DeliveryStatus) error {
	if cursor == nil || cursor.StreamID != status.StreamID || cursor.SessionID != status.SessionID ||
		cursor.IncarnationID != status.IncarnationID || cursor.HarnessGeneration != int64(status.HarnessGeneration) ||
		cursor.ReceivedSequence < 0 || cursor.ProjectedSequence < 0 || cursor.RetiredSequence < 0 ||
		cursor.RetiredSequence > cursor.ProjectedSequence || cursor.ProjectedSequence > cursor.ReceivedSequence {
		return fmt.Errorf("SQL delivery cursor does not match authenticated owner: %w", repoerrors.ErrAgentDeliveryCheckpointMissing)
	}
	return nil
}

func projectedCursorCoveredByPeer(cursor *models.AgentDeliveryCursor, status *agentctl.DeliveryStatus) (uint64, error) {
	if status.Stream == nil {
		if cursor.ProjectedSequence != 0 || cursor.ReceivedSequence != 0 {
			return 0, repoerrors.ErrAgentDeliveryCheckpointMissing
		}
		return 0, nil
	}
	if uint64(cursor.ReceivedSequence)+1 < status.Stream.FirstRetained && status.Stream.FirstRetained <= status.Stream.HighWater+1 {
		return 0, fmt.Errorf("%w: %s", ErrAgentDeliveryHistoryGap, models.AgentDeliveryHistoryGapDetails(status.StreamID, cursor.ReceivedSequence+1, int64(status.Stream.FirstRetained-1)))
	}
	if cursor.ProjectedSequence > int64(status.Stream.HighWater) ||
		status.Stream.Acknowledged > status.Stream.HighWater ||
		(status.Stream.FirstRetained > 0 && uint64(cursor.ProjectedSequence)+1 < status.Stream.FirstRetained) {
		return 0, fmt.Errorf("peer stream no longer covers the SQL projected cursor: %w", repoerrors.ErrAgentDeliveryCheckpointMissing)
	}
	return uint64(cursor.ProjectedSequence), nil
}

func reconcileExpiredAgentDeliveryReplay(
	ctx context.Context,
	execution *AgentExecution,
	event *models.AgentDeliveryEvent,
	repository AgentDeliveryRepository,
	statusReader agentDeliveryStatusReader,
	acknowledger agentDeliveryAcknowledger,
) (uint64, error) {
	if event == nil || statusReader == nil || acknowledger == nil {
		return 0, fmt.Errorf("expired delivery recovery dependencies are unavailable: %w", ErrDurableAdoptionBlocked)
	}
	status, err := statusReader.GetDeliveryStatus(ctx, event.StreamID)
	if err != nil {
		return 0, fmt.Errorf("read authenticated peer delivery state: %w", err)
	}
	projected, err := authenticatedDeliveryReplayCursor(ctx, execution, repository, status)
	if err != nil {
		return 0, err
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, event.StreamID)
	if err != nil {
		return 0, err
	}
	if cursor == nil || event.Sequence > cursor.RetiredSequence || event.Sequence > cursor.ProjectedSequence ||
		projected != uint64(cursor.ProjectedSequence) || status.Stream == nil || projected > status.Stream.HighWater {
		return 0, fmt.Errorf("expired replay is not covered by both retained checkpoints: %w", repoerrors.ErrAgentDeliveryCheckpointMissing)
	}
	if status.Stream.Acknowledged < projected {
		if err := acknowledger.AcknowledgeDelivery(ctx, event.StreamID, projected); err != nil {
			return 0, fmt.Errorf("acknowledge projected delivery cursor after expiry: %w", err)
		}
	}
	return projected, nil
}
