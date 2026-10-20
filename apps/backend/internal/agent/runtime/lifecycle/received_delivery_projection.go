package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// Replay actual inbox payloads through canonical projection and idempotent
// callbacks. An HTTP preflight cannot acknowledge or invent terminal callbacks.
func (sm *StreamManager) projectReceivedDeliveryEvent(ctx context.Context, execution *AgentExecution, delivery AgentDeliveryRepository, retained *models.AgentDeliveryEvent) error {
	var event agentctl.AgentEvent
	if err := json.Unmarshal(retained.Payload, &event); err != nil {
		return fmt.Errorf("decode received delivery event: %w", err)
	}
	if event.Type == "" {
		event.Type = retained.EventType
	}
	if event.Type != retained.EventType {
		return ErrDeliveryOwnerMismatch
	}
	event.DeliveryStreamID = retained.StreamID
	event.DeliveryIncarnationID = retained.IncarnationID
	event.DeliveryHarnessGeneration = uint64(retained.HarnessGeneration)
	event.DeliverySequence = uint64(retained.Sequence)
	event.DeliverySubmissionID = retained.SubmissionID
	if canonicalAgentDeliveryEvent(event) {
		if _, supported := delivery.(canonicalAgentDeliveryProjector); !supported {
			return ErrDeliveryRecoveryBlocked
		}
	}
	effect := deliveryEffectForDeliveryEvent(retained)
	applied, err := sm.deliveryEffectAlreadyApplied(ctx, delivery, effect)
	if err != nil {
		return err
	}
	return sm.processPreparedAgentEvent(ctx, execution, nil, delivery, preparedAgentEvent{
		event: event, durableEvent: retained, deliveryEffect: effect,
		skipAcknowledgement: true, skipCallback: retained.Terminal || applied,
	}, execution.startupAttemptSnapshot())
}
