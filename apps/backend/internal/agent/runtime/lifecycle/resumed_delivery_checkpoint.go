package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// A native conversation's first durable bootstrap can fail before its first
// receipt. Recover the checkpoint by receiving the real first setup event;
// SQL admission still fences the current owner and rejects orphaned receipts.
func (sm *StreamManager) receiveResumedBootstrapCheckpoint(
	ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository,
	client *agentctl.Client, status *agentctl.DeliveryStatus,
) error {
	stream := status.Stream
	if !resumedBootstrapCheckpointEligible(execution, stream) {
		return repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	if _, err := delivery.GetAgentDeliveryCursor(ctx, status.StreamID); !errors.Is(err, sql.ErrNoRows) {
		return repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	if err := validateResumedBootstrapGeneration(ctx, execution, delivery); err != nil {
		return err
	}
	page, replayStream, err := client.ReplayDelivery(ctx, status.StreamID, 0, 1)
	if err != nil {
		return err
	}
	if err := validateRecoveredReplayStream(replayStream, execution, status.StreamID, stream.HighWater); err != nil {
		return err
	}
	if len(page) != 1 || replayStream.FirstRetained != 1 || replayStream.Acknowledged != 0 {
		return repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	receipt, err := resumedBootstrapReceipt(execution, status.StreamID, page[0])
	if err != nil {
		return err
	}
	_, err = delivery.ReceiveAgentDeliveryEvent(ctx, receipt, int64(stream.HighWater))
	return err
}

func resumedBootstrapCheckpointEligible(execution *AgentExecution, stream *journal.Stream) bool {
	return execution.deliveryBootstrap.Load() && execution.DeliveryHarnessGeneration == 1 &&
		execution.ACPSessionID != "" && execution.TaskID != "" && execution.ID != "" &&
		stream != nil && stream.FirstRetained == 1 && stream.Acknowledged == 0 && stream.HighWater > 0
}

func validateResumedBootstrapGeneration(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository) error {
	reader, ok := delivery.(harnessGenerationReader)
	if !ok {
		return repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	generation, err := reader.GetCurrentHarnessSessionGeneration(ctx, execution.SessionID, execution.DeliveryIncarnationID)
	if err != nil {
		return err
	}
	if generation == nil || generation.Generation != 1 || generation.SessionID != execution.SessionID ||
		generation.IncarnationID != execution.DeliveryIncarnationID || generation.NativeSessionID != execution.ACPSessionID {
		return ErrDurableAdoptionIdentityMismatch
	}
	return nil
}

func resumedBootstrapReceipt(execution *AgentExecution, streamID string, first journal.Event) (*models.AgentDeliveryEvent, error) {
	if first.Sequence != 1 || first.SessionID != execution.SessionID || first.StreamID != streamID ||
		first.IncarnationID != execution.DeliveryIncarnationID || first.HarnessGeneration != 1 || first.SubmissionID != "" || first.Terminal {
		return nil, ErrDurableAdoptionIdentityMismatch
	}
	var event agentctl.AgentEvent
	if err := json.Unmarshal(first.Payload, &event); err != nil {
		return nil, err
	}
	if event.Type == "" {
		event.Type = first.Type
	}
	if event.Type != first.Type || event.Type == "" {
		return nil, ErrDurableAdoptionIdentityMismatch
	}
	if _, isTurnContent := turnContentEventTypes[event.Type]; isTurnContent {
		return nil, ErrDurableAdoptionIdentityMismatch
	}
	event.DeliveryStreamID, event.DeliveryIncarnationID = first.StreamID, first.IncarnationID
	event.DeliveryHarnessGeneration, event.DeliverySequence = first.HarnessGeneration, first.Sequence
	event.DeliverySubmissionID = ""
	receipt := durableAgentEvent(execution, event)
	if receipt.Terminal {
		return nil, ErrDurableAdoptionIdentityMismatch
	}
	// Preflight precedes capability negotiation, so bind the verified owner
	// explicitly instead of relying on the not-yet-published delivery mode.
	receipt.TaskID, receipt.ExecutionID = execution.TaskID, execution.ID
	receipt.OwnerGeneration, receipt.ExpectedStreamID = 1, streamID
	return receipt, nil
}
