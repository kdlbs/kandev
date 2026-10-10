package lifecycle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// AgentDeliveryRepository is the backend half of the retained agentctl
// stream. The lifecycle package depends on the narrow protocol, not the SQL
// implementation, so isolated runtimes and tests can provide another store.
type AgentDeliveryRepository interface {
	ReceiveAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, remoteHighWater int64) (bool, error)
	GetAgentDeliveryCursor(ctx context.Context, streamID string) (*models.AgentDeliveryCursor, error)
	ProjectAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, effect *models.AgentDeliveryEffect) (bool, error)
}

// canonicalAgentDeliveryProjector is implemented by the backend repository
// that owns task messages. It must persist the canonical message and advance
// the inbox cursor in one transaction before returning.
type canonicalAgentDeliveryProjector interface {
	ProjectCanonicalAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, effect *models.AgentDeliveryEffect) (bool, error)
}

type canonicalAgentDeliveryBatchProjector interface {
	ProjectCanonicalAgentDeliveryEvents(ctx context.Context, events []*models.AgentDeliveryEvent, effects []*models.AgentDeliveryEffect) ([]bool, error)
}

type agentDeliveryAcknowledger interface {
	AcknowledgeDelivery(ctx context.Context, streamID string, sequence uint64) error
}

type agentDeliveryEffectReader interface {
	GetAgentDeliveryEffect(ctx context.Context, effectKey string) (*models.AgentDeliveryEffect, error)
}

type agentDeliveryTerminalSettler interface {
	SettleAgentDeliveryTerminal(context.Context, string, int64, models.DeliverySubmissionState, time.Time) (bool, error)
}

type harnessGenerationReader interface {
	GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error)
}

var errAgentDeliveryCursorUnavailable = errors.New("agent delivery cursor is unavailable")

const durableDeliveryReplayPageSize = 1000

// ReplayRecoveredDelivery drains the retained durable stream through the same
// inbox, canonical projection, lifecycle, and acknowledgement pipeline used
// by live WebSocket events. The descriptor's captured high-water mark is the
// completion boundary; a page of 1000 events is only one page and never means
// that replay is complete.
func (sm *StreamManager) ReplayRecoveredDelivery(ctx context.Context, execution *AgentExecution) error {
	if execution == nil || execution.DeliveryMode != DurableDeliveryV1 {
		return nil
	}
	descriptor := execution.DeliveryDescriptor
	if descriptor == nil || descriptor.Stream == nil || descriptor.Stream.HighWater == 0 {
		return nil
	}
	streamID := execution.DeliveryStreamID
	if streamID == "" || descriptor.Stream.StreamID != streamID {
		return fmt.Errorf("durable recovery stream identity is missing or inconsistent")
	}
	delivery := sm.deliveryRepository()
	if delivery == nil {
		return errors.New("durable recovery repository is unavailable")
	}
	after := execution.DeliveryReplayCursor
	target := descriptor.Stream.HighWater
	if after > target {
		return fmt.Errorf("durable recovery cursor %d is ahead of captured high-water %d", after, target)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return errors.New("durable recovery agentctl client is unavailable")
	}
	defer releaseClient()

	startupGeneration := execution.startupAttemptSnapshot()
	for after < target {
		next, err := sm.replayRecoveredDeliveryPage(
			ctx, execution, client, delivery, streamID, after, target, startupGeneration,
		)
		if err != nil {
			return err
		}
		after = next
		execution.DeliveryReplayCursor = after
	}
	return nil
}

func (sm *StreamManager) replayRecoveredDeliveryPage(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	delivery AgentDeliveryRepository,
	streamID string,
	after, target, startupGeneration uint64,
) (uint64, error) {
	remaining := target - after
	limit := durableDeliveryReplayPageSize
	if remaining < uint64(limit) {
		limit = int(remaining)
	}
	page, stream, err := client.ReplayDelivery(ctx, streamID, after, limit)
	if err != nil {
		return after, fmt.Errorf("replay durable delivery after sequence %d: %w", after, err)
	}
	if err := validateRecoveredReplayStream(stream, execution, streamID, target); err != nil {
		return after, err
	}
	if len(page) == 0 {
		return after, fmt.Errorf("durable replay has a gap after sequence %d before high-water %d", after, target)
	}
	start := after
	for _, committed := range page {
		if committed.Sequence > target {
			break
		}
		if committed.Sequence != after+1 {
			return after, fmt.Errorf("durable replay expected sequence %d, received %d", after+1, committed.Sequence)
		}
		if err := sm.processRecoveredDeliveryEvent(ctx, execution, client, delivery, committed, startupGeneration); err != nil {
			return after, err
		}
		after = committed.Sequence
	}
	if after == start {
		return after, fmt.Errorf("durable replay made no progress after sequence %d", after)
	}
	return after, nil
}

func validateRecoveredReplayStream(
	stream journal.Stream,
	execution *AgentExecution,
	streamID string,
	target uint64,
) error {
	if stream.StreamID != streamID || stream.SessionID != execution.SessionID ||
		stream.IncarnationID != execution.DeliveryIncarnationID ||
		stream.HarnessGeneration != execution.DeliveryHarnessGeneration ||
		stream.HighWater < target {
		return errors.New("durable replay returned an inconsistent stream owner or high-water mark")
	}
	return nil
}

func (sm *StreamManager) processRecoveredDeliveryEvent(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	delivery AgentDeliveryRepository,
	committed journal.Event,
	startupGeneration uint64,
) error {
	var event agentctl.AgentEvent
	if err := json.Unmarshal(committed.Payload, &event); err != nil {
		return fmt.Errorf("decode durable replay sequence %d: %w", committed.Sequence, err)
	}
	if event.Type == "" {
		event.Type = committed.Type
	}
	event.DeliveryStreamID = committed.StreamID
	event.DeliveryIncarnationID = committed.IncarnationID
	event.DeliveryHarnessGeneration = committed.HarnessGeneration
	event.DeliverySequence = committed.Sequence
	event.DeliverySubmissionID = committed.SubmissionID
	if err := sm.processAgentEvent(ctx, execution, client, delivery, event, startupGeneration); err != nil {
		return fmt.Errorf("process durable replay sequence %d: %w", committed.Sequence, err)
	}
	return nil
}

// reconcileDisconnectedSubmission runs the same bounded operation used by
// user-triggered retry. The old socket context has already ended, so this
// state-only query is detached from that context but still follows manager
// shutdown and per-execution Stop cancellation.
func (sm *StreamManager) reconcileDisconnectedSubmission(
	ctx context.Context,
	execution *AgentExecution,
) DeliveryReconciliationResult {
	return sm.ReconcileAgentDelivery(context.WithoutCancel(ctx), execution)
}

func (sm *StreamManager) setAgentDeliveryRepository(repository AgentDeliveryRepository) {
	sm.deliveryMu.Lock()
	sm.delivery = repository
	sm.deliverySubmissionSettler = nil
	if settler, ok := repository.(agentDeliveryTerminalSettler); ok {
		sm.deliverySubmissionSettler = func(
			ctx context.Context,
			_ *AgentExecution,
			identity DeliveryReconciliationIdentity,
			submission *journal.Submission,
		) (bool, error) {
			return settleProjectedDeliveryTerminal(ctx, repository, settler, identity, submission)
		}
	}
	sm.deliveryMu.Unlock()
}

func settleProjectedDeliveryTerminal(
	ctx context.Context,
	repository AgentDeliveryRepository,
	settler agentDeliveryTerminalSettler,
	identity DeliveryReconciliationIdentity,
	submission *journal.Submission,
) (bool, error) {
	if !submissionHasRetainedTerminal(submission) || submission.TerminalSequence == 0 {
		return false, nil
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, identity.StreamID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if cursor == nil || cursor.ProjectedSequence < int64(submission.TerminalSequence) {
		return false, nil
	}
	return settler.SettleAgentDeliveryTerminal(
		ctx,
		identity.StreamID,
		int64(submission.TerminalSequence),
		models.DeliverySubmissionState(submission.State),
		time.Now().UTC(),
	)
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
	if event.DeliverySequence > math.MaxInt64 {
		return false, fmt.Errorf("delivery sequence %d exceeds backend range", event.DeliverySequence)
	}
	return sm.receiveDurableAgentDeliveryEvent(ctx, durableAgentEvent(execution, event), repository)
}

func (sm *StreamManager) receiveDurableAgentDeliveryEvent(
	ctx context.Context,
	deliveryEvent *models.AgentDeliveryEvent,
	repository AgentDeliveryRepository,
) (bool, error) {
	if deliveryEvent == nil || deliveryEvent.StreamID == "" || deliveryEvent.Sequence == 0 {
		return true, nil
	}
	if deliveryEvent.Sequence < 0 {
		return false, fmt.Errorf("delivery sequence %d exceeds backend range", deliveryEvent.Sequence)
	}
	inserted, err := repository.ReceiveAgentDeliveryEvent(ctx, deliveryEvent, 0)
	if err != nil {
		return false, err
	}
	if inserted {
		return true, nil
	}
	cursor, err := repository.GetAgentDeliveryCursor(ctx, deliveryEvent.StreamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	return cursor == nil || cursor.ProjectedSequence < deliveryEvent.Sequence, nil
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
	return sm.projectDurableAgentDeliveryEventWithEffect(
		ctx, durableAgentEvent(execution, event), repository, effect,
	)
}

func (sm *StreamManager) projectDurableAgentDeliveryEventWithEffect(
	ctx context.Context,
	deliveryEvent *models.AgentDeliveryEvent,
	repository AgentDeliveryRepository,
	effect *models.AgentDeliveryEffect,
) error {
	if deliveryEvent == nil || deliveryEvent.StreamID == "" || deliveryEvent.Sequence == 0 {
		return nil
	}
	_, returnError := repository.ProjectAgentDeliveryEvent(ctx, deliveryEvent, effect)
	if returnError != nil {
		return returnError
	}
	if !deliveryEvent.Terminal || deliveryEvent.SubmissionID == "" {
		return nil
	}
	outcome := projectedDeliveryTerminalOutcome(deliveryEvent)
	if outcome == "" {
		return nil
	}
	settler, ok := repository.(agentDeliveryTerminalSettler)
	if !ok {
		return nil
	}
	_, err := settler.SettleAgentDeliveryTerminal(ctx, deliveryEvent.StreamID, deliveryEvent.Sequence, outcome, time.Now().UTC())
	return err
}

func projectedDeliveryTerminalOutcome(event *models.AgentDeliveryEvent) models.DeliverySubmissionState {
	switch event.EventType {
	case streams.EventTypeComplete:
		var payload streams.AgentEvent
		_ = json.Unmarshal(event.Payload, &payload)
		if payload.Data["stop_reason"] == string(models.DeliverySubmissionCancelled) {
			return models.DeliverySubmissionCancelled
		}
		return models.DeliverySubmissionCompleted
	case streams.EventTypeError:
		return models.DeliverySubmissionFailed
	case string(models.DeliverySubmissionCancelled), "canceled":
		return models.DeliverySubmissionCancelled
	default:
		return ""
	}
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
	deliveryEvent := durableAgentEvent(execution, event)
	if err := sm.projectDurableAgentDeliveryEventWithEffect(ctx, deliveryEvent, repository, effect); err != nil {
		return err
	}
	return acknowledgeDurableAgentEvent(ctx, event, acknowledger)
}

func (sm *StreamManager) projectAndAcknowledgeDurableAgentDeliveryEventWithEffect(
	ctx context.Context,
	deliveryEvent *models.AgentDeliveryEvent,
	event agentctl.AgentEvent,
	repository AgentDeliveryRepository,
	acknowledger agentDeliveryAcknowledger,
	effect *models.AgentDeliveryEffect,
) error {
	if err := sm.projectDurableAgentDeliveryEventWithEffect(ctx, deliveryEvent, repository, effect); err != nil {
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
	// Event projection never claims the orchestrator's workflow transition key.
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

func deliveryEffectForDeliveryEvent(event *models.AgentDeliveryEvent) *models.AgentDeliveryEffect {
	if event == nil || event.StreamID == "" || event.Sequence == 0 {
		return nil
	}
	effectKey := fmt.Sprintf("agent_delivery.event:%s:%d", event.StreamID, event.Sequence)
	effectType := "agent_delivery.event"
	// Event projection never claims the orchestrator's workflow transition key.
	now := time.Now().UTC()
	return &models.AgentDeliveryEffect{
		EffectKey:   effectKey,
		StreamID:    event.StreamID,
		Sequence:    event.Sequence,
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
) (bool, error) {
	if effect == nil {
		return false, nil
	}
	reader, ok := repository.(agentDeliveryEffectReader)
	if !ok {
		return false, nil
	}
	stored, err := reader.GetAgentDeliveryEffect(ctx, effect.EffectKey)
	if errors.Is(err, repoerrors.ErrAgentDeliveryEffectNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read durable delivery effect %q: %w", effect.EffectKey, err)
	}
	return stored != nil && stored.State == models.DeliveryEffectCompleted, nil
}

func deliveryEventIsStale(
	ctx context.Context,
	execution *AgentExecution,
	event *models.AgentDeliveryEvent,
	repository AgentDeliveryRepository,
) (bool, error) {
	if event == nil || event.HarnessGeneration == 0 || event.IncarnationID == "" {
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
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, event.IncarnationID)
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return current != nil && current.Generation > event.HarnessGeneration, nil
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
	if payloadEvent.TurnID == "" && execution != nil {
		payloadEvent.TurnID = execution.promptTurnIDSnapshot()
	}
	payloadEvent.CanonicalMessageID = canonicalAgentMessageID(execution, event)
	payloadEvent.CanonicalProjection = false
	payloadEvent.CanonicalMessageAppend = false
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

func canonicalAgentDeliveryEvent(event agentctl.AgentEvent) bool {
	return event.Type == streams.EventTypeMessageChunk || event.Type == streams.EventTypeReasoning
}

func canonicalMessageType(event agentctl.AgentEvent) string {
	if event.Type == streams.EventTypeReasoning {
		return "thinking"
	}
	return "message"
}

func canonicalAgentMessageID(execution *AgentExecution, event agentctl.AgentEvent) string {
	if event.CanonicalMessageID != "" {
		return event.CanonicalMessageID
	}
	streamID := event.DeliveryStreamID
	if streamID == "" && execution != nil {
		streamID = execution.DeliveryStreamID
	}
	if streamID == "" && execution != nil {
		streamID = execution.SessionID
	}
	turnID := event.TurnID
	if turnID == "" && execution != nil {
		turnID = execution.promptTurnIDSnapshot()
	}
	scope := event.ProtocolMessageID
	if scope == "" {
		scope = turnID
	}
	if scope == "" {
		scope = event.DeliverySubmissionID
	}
	if scope == "" {
		scope = "prompt:" + strconv.FormatUint(event.PromptGeneration, 10)
	}
	if scope == "prompt:0" {
		scope = "stream"
	}
	digest := sha256.Sum256([]byte(streamID + ":" + event.Type + ":" + scope))
	return "agent-delivery-" + fmt.Sprintf("%x", digest[:16])
}
