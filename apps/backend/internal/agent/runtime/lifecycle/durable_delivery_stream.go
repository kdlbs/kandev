package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

// AgentDeliveryRepository is the backend half of the retained agentctl
// stream. The lifecycle package depends on the narrow protocol, not the SQL
// implementation, so isolated runtimes and tests can provide another store.
type AgentDeliveryRepository interface {
	ReceiveAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, remoteHighWater int64) (bool, error)
	GetAgentDeliveryCursor(ctx context.Context, streamID string) (*models.AgentDeliveryCursor, error)
	ProjectAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, effect *models.AgentDeliveryEffect) (bool, error)
}

type agentDeliveryAcknowledger interface {
	AcknowledgeDelivery(ctx context.Context, streamID string, sequence uint64) error
}

type agentDeliveryEffectReader interface {
	GetAgentDeliveryEffect(ctx context.Context, effectKey string) (*models.AgentDeliveryEffect, error)
}

type harnessGenerationReader interface {
	GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error)
}

const deliveryReconciliationTimeout = 3 * time.Second

var errAgentDeliveryCursorUnavailable = errors.New("agent delivery cursor is unavailable")

// reconcileDisconnectedSubmission performs one bounded state query before the
// ordinary disconnect failure path. Dispatch completion only means that the
// prompt call was accepted by the harness. Recovery is safe only after the
// journal has retained a terminal event and the backend cursor is still behind
// it, so the reconnect can replay the actual terminal outcome. An accepted
// prompt without that evidence remains uncertain and is never resent.
func (sm *StreamManager) reconcileDisconnectedSubmission(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
) bool {
	if execution == nil || client == nil || execution.deliverySubmissionIDSnapshot() == "" {
		return false
	}
	submissionID := execution.deliverySubmissionIDSnapshot()
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deliveryReconciliationTimeout)
	defer cancel()
	submission, err := client.GetDeliverySubmission(reconcileCtx, submissionID)
	if err != nil {
		sm.logger.Warn("durable prompt submission reconciliation failed",
			zap.String("execution_id", execution.ID),
			zap.String("submission_id", submissionID),
			zap.Error(err))
		return false
	}
	if submission.State != journal.SubmissionCompleted ||
		!submission.TerminalEventRetained || submission.TerminalSequence == 0 {
		return false
	}
	delivery := sm.deliveryRepository()
	if delivery == nil {
		return false
	}
	streamID := execution.DeliveryStreamID
	if streamID == "" {
		streamID = execution.SessionID
	}
	cursor, cursorErr := delivery.GetAgentDeliveryCursor(reconcileCtx, streamID)
	if cursorErr != nil && !errors.Is(cursorErr, sql.ErrNoRows) {
		sm.logger.Warn("durable prompt reconciliation could not verify projected terminal event",
			zap.String("execution_id", execution.ID),
			zap.String("submission_id", submissionID),
			zap.Error(cursorErr))
		return false
	}
	if cursor != nil && cursor.ProjectedSequence >= int64(submission.TerminalSequence) {
		// The canonical projection already passed the terminal event. There is
		// no retained outcome here that can safely reconstruct a missing
		// lifecycle signal, so leave the execution in the explicit recovery path.
		return false
	}
	// The stream reader has already detached this connection before invoking
	// its disconnect callback. Reconnect asynchronously so committed terminal
	// events are replayed from the backend's projected cursor.
	sm.connectUpdatesStreamAsync(execution, nil)
	sm.logger.Info("reconciled completed durable prompt after stream disconnect",
		zap.String("execution_id", execution.ID),
		zap.String("submission_id", submissionID))
	return true
}

func (sm *StreamManager) setAgentDeliveryRepository(repository AgentDeliveryRepository) {
	sm.deliveryMu.Lock()
	sm.delivery = repository
	sm.deliveryMu.Unlock()
}

func (sm *StreamManager) deliveryRepository() AgentDeliveryRepository {
	sm.deliveryMu.RLock()
	defer sm.deliveryMu.RUnlock()
	return sm.delivery
}

func (sm *StreamManager) deliveryReplayCursor(ctx context.Context, repository AgentDeliveryRepository, streamID string) (uint64, error) {
	if repository == nil || streamID == "" {
		return 0, nil
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, streamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("load agent delivery cursor: %w", err)
	}
	if cursor == nil {
		return 0, errAgentDeliveryCursorUnavailable
	}
	if cursor.ProjectedSequence <= 0 {
		return 0, nil
	}
	return uint64(cursor.ProjectedSequence), nil
}

func (sm *StreamManager) receiveDurableAgentEvent(
	ctx context.Context,
	execution *AgentExecution,
	event agentctl.AgentEvent,
	repository AgentDeliveryRepository,
) (bool, error) {
	if event.DeliveryStreamID == "" || event.DeliverySequence == 0 {
		return true, nil
	}
	if event.DeliverySequence > math.MaxInt64 {
		return false, fmt.Errorf("delivery sequence %d exceeds backend range", event.DeliverySequence)
	}
	deliveryEvent := durableAgentEvent(execution, event)
	inserted, err := repository.ReceiveAgentDeliveryEvent(ctx, deliveryEvent, 0)
	if err != nil {
		return false, err
	}
	if inserted {
		return true, nil
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, event.DeliveryStreamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	return cursor == nil || cursor.ProjectedSequence < int64(event.DeliverySequence), nil
}

func (sm *StreamManager) projectDurableAgentEvent(
	ctx context.Context,
	execution *AgentExecution,
	event agentctl.AgentEvent,
	repository AgentDeliveryRepository,
) error {
	return sm.projectDurableAgentEventWithEffect(ctx, execution, event, repository, deliveryEffectForEvent(event))
}

func (sm *StreamManager) projectDurableAgentEventWithEffect(
	ctx context.Context,
	execution *AgentExecution,
	event agentctl.AgentEvent,
	repository AgentDeliveryRepository,
	effect *models.AgentDeliveryEffect,
) error {
	if event.DeliveryStreamID == "" || event.DeliverySequence == 0 {
		return nil
	}
	deliveryEvent := durableAgentEvent(execution, event)
	_, returnError := repository.ProjectAgentDeliveryEvent(ctx, deliveryEvent, effect)
	return returnError
}

func (sm *StreamManager) projectAndAcknowledgeDurableAgentEvent(
	ctx context.Context,
	execution *AgentExecution,
	event agentctl.AgentEvent,
	repository AgentDeliveryRepository,
	acknowledger agentDeliveryAcknowledger,
) error {
	return sm.projectAndAcknowledgeDurableAgentEventWithEffect(
		ctx, execution, event, repository, acknowledger, deliveryEffectForEvent(event),
	)
}

func (sm *StreamManager) projectAndAcknowledgeDurableAgentEventWithEffect(
	ctx context.Context,
	execution *AgentExecution,
	event agentctl.AgentEvent,
	repository AgentDeliveryRepository,
	acknowledger agentDeliveryAcknowledger,
	effect *models.AgentDeliveryEffect,
) error {
	if err := sm.projectDurableAgentEventWithEffect(ctx, execution, event, repository, effect); err != nil {
		return err
	}
	return acknowledgeDurableAgentEvent(ctx, event, acknowledger)
}

func deliveryEffectForEvent(event agentctl.AgentEvent) *models.AgentDeliveryEffect {
	if event.DeliveryStreamID == "" || event.DeliverySequence == 0 {
		return nil
	}
	effectKey := fmt.Sprintf("agent_delivery.event:%s:%d", event.DeliveryStreamID, event.DeliverySequence)
	effectType := "agent_delivery.event"
	if (event.Type == streams.EventTypeComplete || event.Type == streams.EventTypeError) && event.TurnID != "" {
		effectKey = "workflow.on_turn_complete:" + event.TurnID
		effectType = "workflow.on_turn_complete"
	}
	now := time.Now().UTC()
	return &models.AgentDeliveryEffect{
		EffectKey:   effectKey,
		StreamID:    event.DeliveryStreamID,
		Sequence:    int64(event.DeliverySequence),
		EffectType:  effectType,
		State:       models.DeliveryEffectCompleted,
		CreatedAt:   now,
		CompletedAt: &now,
	}
}

func (sm *StreamManager) deliveryEffectAlreadyApplied(
	ctx context.Context,
	repository AgentDeliveryRepository,
	effect *models.AgentDeliveryEffect,
) bool {
	if effect == nil {
		return false
	}
	reader, ok := repository.(agentDeliveryEffectReader)
	if !ok {
		return false
	}
	stored, err := reader.GetAgentDeliveryEffect(ctx, effect.EffectKey)
	if errors.Is(err, repoerrors.ErrAgentDeliveryEffectNotFound) {
		return false
	}
	if err != nil {
		sm.logger.Warn("failed to read durable delivery effect; suppressing replay",
			zap.String("effect_key", effect.EffectKey), zap.Error(err))
		return true
	}
	return stored != nil && stored.State == models.DeliveryEffectCompleted
}

func deliveryEventIsStale(
	ctx context.Context,
	execution *AgentExecution,
	event agentctl.AgentEvent,
	repository AgentDeliveryRepository,
) (bool, error) {
	if event.DeliveryHarnessGeneration == 0 || event.DeliveryIncarnationID == "" {
		return false, nil
	}
	reader, ok := repository.(harnessGenerationReader)
	if !ok {
		return false, nil
	}
	sessionID := event.SessionID
	if execution != nil && execution.SessionID != "" {
		sessionID = execution.SessionID
	}
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, event.DeliveryIncarnationID)
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return current != nil && current.Generation > int64(event.DeliveryHarnessGeneration), nil
}

func acknowledgeDurableAgentEvent(ctx context.Context, event agentctl.AgentEvent, acknowledger agentDeliveryAcknowledger) error {
	if event.DeliveryStreamID == "" || event.DeliverySequence == 0 {
		return nil
	}
	if acknowledger == nil {
		return errors.New("agent delivery acknowledger is unavailable")
	}
	if err := acknowledger.AcknowledgeDelivery(ctx, event.DeliveryStreamID, event.DeliverySequence); err != nil {
		return fmt.Errorf("acknowledge durable agent event: %w", err)
	}
	return nil
}

func durableAgentEvent(execution *AgentExecution, event agentctl.AgentEvent) *models.AgentDeliveryEvent {
	sessionID := ""
	if execution != nil {
		sessionID = execution.SessionID
	}
	if sessionID == "" {
		sessionID = event.SessionID
	}
	incarnationID := event.DeliveryIncarnationID
	if incarnationID == "" && execution != nil {
		incarnationID = execution.DeliveryIncarnationID
	}
	if incarnationID == "" {
		incarnationID = event.DeliveryStreamID
	}
	harnessGeneration := event.DeliveryHarnessGeneration
	if harnessGeneration == 0 && execution != nil {
		harnessGeneration = execution.DeliveryHarnessGeneration
	}
	if harnessGeneration == 0 {
		harnessGeneration = 1
	}
	payloadEvent := event
	payloadEvent.DeliveryStreamID = ""
	payloadEvent.DeliveryIncarnationID = ""
	payloadEvent.DeliveryHarnessGeneration = 0
	payloadEvent.DeliverySequence = 0
	payloadEvent.DeliverySubmissionID = ""
	payload, _ := json.Marshal(payloadEvent)
	return &models.AgentDeliveryEvent{
		SessionID:         sessionID,
		IncarnationID:     incarnationID,
		HarnessGeneration: int64(harnessGeneration),
		StreamID:          event.DeliveryStreamID,
		Sequence:          int64(event.DeliverySequence),
		SubmissionID:      event.DeliverySubmissionID,
		EventType:         event.Type,
		Payload:           payload,
		Terminal:          event.Type == streams.EventTypeComplete || event.Type == streams.EventTypeError,
	}
}
