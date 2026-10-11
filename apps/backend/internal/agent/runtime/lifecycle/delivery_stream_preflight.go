package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

type deliveryStreamFailure struct {
	generation uint64
	err        error
}

func (e *AgentExecution) recordDeliveryStreamFailure(generation uint64, err error) {
	if e.startupAttemptSnapshot() == generation {
		e.deliveryStreamConnectError.Store(&deliveryStreamFailure{generation: generation, err: err})
	}
}

func (e *AgentExecution) deliveryStreamFailureSnapshot() error {
	if failure := e.deliveryStreamConnectError.Load(); failure != nil && failure.generation == e.startupAttemptSnapshot() {
		return failure.err
	}
	return nil
}

func (sm *StreamManager) initialDeliveryReplayCursor(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, client *agentctl.Client, streamID string) (uint64, error) {
	after, err := sm.deliveryReplayCursor(ctx, delivery, streamID)
	if err != nil || delivery == nil || execution.DeliveryIncarnationID == "" || execution.DeliveryHarnessGeneration == 0 {
		return after, err
	}
	status, err := client.GetDeliveryStatus(ctx, streamID)
	if err != nil {
		if canUseLegacyEmptyDelivery(ctx, execution, delivery, streamID, err) {
			return after, nil
		}
		return 0, fmt.Errorf("read authenticated delivery descriptor: %w", err)
	}
	if canUseLegacyStorageDescriptor(ctx, execution, delivery, streamID, status) {
		return 0, nil
	}
	if err := validateAuthenticatedDeliveryDescriptor(execution, status); err != nil {
		return 0, err
	}
	if err := validateRecoveredStream(status); err != nil {
		return 0, err
	}
	if stream := status.Stream; stream != nil && (stream.HighWater > 1<<63-1 || stream.Acknowledged > stream.HighWater || stream.FirstRetained > stream.HighWater+1) {
		return 0, ErrDeliveryOwnerMismatch
	}
	if err := sm.projectReceivedDeliveryPrefix(ctx, execution, delivery, status); err != nil {
		return 0, err
	}
	return authenticatedDeliveryReplayCursor(ctx, execution, delivery, status)
}

// A first child can exit before ACP initialization commits the native owner.
// Replay its retained events from zero; the existing bootstrap processor waits
// for committed authority before writing receipts or acknowledgments.
func canReplayFreshBootstrapEvents(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, status *agentctl.DeliveryStatus) bool {
	stream := status.Stream
	if !execution.deliveryBootstrap.Load() || execution.ACPSessionID != "" || execution.RequiredNativeConversationID != "" ||
		execution.isResumedSession || execution.DeliveryHarnessGeneration != 1 || stream == nil || stream.FirstRetained != 1 ||
		stream.Acknowledged != 0 || status.Unresolved || status.SubmissionCount != 0 || len(status.Submissions) != 0 || status.SubmissionsTruncated {
		return false
	}
	if _, err := delivery.GetAgentDeliveryCursor(ctx, status.StreamID); !errors.Is(err, sql.ErrNoRows) {
		return false
	}
	reader, supported := delivery.(harnessGenerationReader)
	if !supported {
		return false
	}
	_, err := reader.GetCurrentHarnessSessionGeneration(ctx, execution.SessionID, execution.DeliveryIncarnationID)
	return errors.Is(err, models.ErrTaskSessionNotFound)
}

func canUseLegacyEmptyDelivery(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, streamID string, err error) bool {
	var response *agentctl.DeliveryHTTPError
	if execution.DeliveryMode == DurableDeliveryV1 || !errors.As(err, &response) ||
		(response.StatusCode != http.StatusNotFound && response.StatusCode != http.StatusNotImplemented) {
		return false
	}
	_, cursorErr := delivery.GetAgentDeliveryCursor(ctx, streamID)
	return errors.Is(cursorErr, sql.ErrNoRows)
}

type agentDeliveryInboxReader interface {
	ListUnprojectedAgentDeliveryEvents(context.Context, string, int) ([]*models.AgentDeliveryEvent, error)
}

func (sm *StreamManager) projectReceivedDeliveryPrefix(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, status *agentctl.DeliveryStatus) error {
	cursor, err := delivery.GetAgentDeliveryCursor(ctx, status.StreamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateAuthenticatedSQLDeliveryCursor(cursor, status); err != nil {
		return err
	}
	stream := status.Stream
	if stream != nil && uint64(cursor.ReceivedSequence) > stream.HighWater {
		return ErrDeliveryOwnerMismatch
	}
	if stream == nil || uint64(cursor.ProjectedSequence)+1 >= stream.FirstRetained || uint64(cursor.ReceivedSequence)+1 < stream.FirstRetained {
		return nil
	}
	reader, supported := delivery.(agentDeliveryInboxReader)
	if !supported {
		return ErrDeliveryRecoveryBlocked
	}
	for cursor.ProjectedSequence < cursor.ReceivedSequence {
		events, err := reader.ListUnprojectedAgentDeliveryEvents(ctx, status.StreamID, durableDeliveryReplayPageSize)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return ErrDeliveryRecoveryBlocked
		}
		if err := sm.projectReceivedDeliveryPage(ctx, execution, delivery, cursor, events); err != nil {
			return err
		}
	}
	return nil
}

func (sm *StreamManager) projectReceivedDeliveryPage(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, cursor *models.AgentDeliveryCursor, events []*models.AgentDeliveryEvent) error {
	for _, event := range events {
		if event.Sequence != cursor.ProjectedSequence+1 || event.Sequence > cursor.ReceivedSequence || event.StreamID != cursor.StreamID ||
			event.SessionID != execution.SessionID || event.IncarnationID != execution.DeliveryIncarnationID || event.HarnessGeneration != int64(execution.DeliveryHarnessGeneration) {
			return ErrDeliveryOwnerMismatch
		}
		if err := sm.projectReceivedDeliveryEvent(ctx, execution, delivery, event); err != nil {
			return err
		}
		cursor.ProjectedSequence = event.Sequence
	}
	return nil
}

func canUseLegacyStorageDescriptor(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, streamID string, status *agentctl.DeliveryStatus) bool {
	if status.Durable || status.Reason != durableDeliveryStorageNotDurable || execution.DeliveryMode == DurableDeliveryV1 {
		return false
	}
	_, err := delivery.GetAgentDeliveryCursor(ctx, streamID)
	return errors.Is(err, sql.ErrNoRows)
}
