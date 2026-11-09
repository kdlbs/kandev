package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type preparedAgentEvent struct {
	event               agentctl.AgentEvent
	durableEvent        *models.AgentDeliveryEvent
	deliveryEffect      *models.AgentDeliveryEffect
	receiptDeferred     bool
	skipCallback        bool
	skipAcknowledgement bool
	staleTerminal       bool
	duplicate           bool
}

func (sm *StreamManager) processAgentEvent(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	delivery AgentDeliveryRepository,
	event agentctl.AgentEvent,
	startupGeneration uint64,
) error {
	prepared, err := sm.prepareAgentEvent(ctx, execution, client, delivery, event)
	if err != nil {
		return err
	}
	return sm.processPreparedAgentEvent(ctx, execution, client, delivery, prepared, startupGeneration)
}

func (sm *StreamManager) prepareAgentEvent(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	delivery AgentDeliveryRepository,
	event agentctl.AgentEvent,
) (preparedAgentEvent, error) {
	return sm.prepareAgentEventWithReceipt(
		ctx, execution, client, delivery, event, false, execution.deliveryStartupSnapshot(),
	)
}

// prepareAgentEventWithReceipt defers the inbox receipt for an event that the
// stream processor will commit with a compatible batch. The returned prepared
// event then carries no inbox row until the batch transaction runs.
func (sm *StreamManager) prepareAgentEventWithReceipt(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	delivery AgentDeliveryRepository,
	event agentctl.AgentEvent,
	deferReceipt bool,
	attempt *deliveryStartupAttempt,
) (preparedAgentEvent, error) {
	if err := attempt.awaitNegotiation(ctx); err != nil {
		return preparedAgentEvent{}, err
	}
	durable, err := durableAgentEventIdentity(execution, event)
	if err != nil {
		return preparedAgentEvent{}, err
	}
	if !shellOutputEventAdmissible(event, durable && delivery != nil) {
		return preparedAgentEvent{}, ErrDeliveryRecoveryBlocked
	}
	if !durable || delivery == nil {
		return preparedLegacyAgentEvent(execution, event), nil
	}

	// Capture the durable record before invoking lifecycle callbacks. Those
	// callbacks can advance or clear prompt state, but the inbox payload must
	// remain byte-identical from admission through projection.
	durableEvent := durableAgentEvent(execution, event)
	// A replayed event keeps its own prompt-attempt identity. Do not let it
	// overwrite the live execution's submission before completion claims run.
	staleSubmission := event.Recovered && deliverySubmissionIdentityMismatch(execution, event.DeliverySubmissionID)
	staleTerminal := staleSubmission && durableEvent.Terminal
	process, skipCallback, effect, deferred, err := sm.prepareDurableAgentDeliveryEvent(
		ctx, execution, durableEvent, delivery, client, deferReceipt, attempt,
	)
	if err != nil {
		return preparedAgentEvent{}, fmt.Errorf("durable agent event ownership check: %w", err)
	}
	if event.DeliverySubmissionID != "" && !staleSubmission {
		execution.setDeliverySubmissionID(event.DeliverySubmissionID)
	}
	if !process {
		if err := sm.acknowledgeDuplicateDurableEvent(ctx, client, event); err != nil {
			return preparedAgentEvent{}, err
		}
		return preparedAgentEvent{event: event, duplicate: true}, nil
	}
	return preparedAgentEvent{
		event:           event,
		durableEvent:    durableEvent,
		deliveryEffect:  effect,
		receiptDeferred: deferred,
		skipCallback:    skipCallback,
		staleTerminal:   staleTerminal,
	}, nil
}

// shellOutputEventAdmissible reports whether shell-output payload and event
// type agree, and whether a shell-output update has the durable delivery it
// requires.
func shellOutputEventAdmissible(event agentctl.AgentEvent, durableDelivery bool) bool {
	if event.Type == streams.EventTypeShellOutputUpdate {
		return durableDelivery && event.ShellOutput != nil
	}
	return event.ShellOutput == nil
}

func preparedLegacyAgentEvent(execution *AgentExecution, event agentctl.AgentEvent) preparedAgentEvent {
	if event.DeliverySubmissionID != "" {
		execution.setDeliverySubmissionID(event.DeliverySubmissionID)
	}
	return preparedAgentEvent{event: event}
}

func (sm *StreamManager) acknowledgeDuplicateDurableEvent(
	ctx context.Context,
	client *agentctl.Client,
	event agentctl.AgentEvent,
) error {
	if err := sm.deliveryJournalReleaser().acknowledge(
		ctx, client, event.DeliveryStreamID, event.DeliverySequence,
	); err != nil {
		return fmt.Errorf("acknowledge duplicate durable event: %w", err)
	}
	return nil
}

func (sm *StreamManager) notifyAgentEvent(
	execution *AgentExecution,
	event agentctl.AgentEvent,
	startupGeneration uint64,
) {
	if sm.callbacks.OnAgentEventWithGeneration != nil {
		sm.callbacks.OnAgentEventWithGeneration(execution, event, startupGeneration)
		return
	}
	if sm.callbacks.OnAgentEvent != nil {
		sm.callbacks.OnAgentEvent(execution, event)
	}
}

func (sm *StreamManager) prepareDurableAgentDeliveryEvent(
	ctx context.Context,
	execution *AgentExecution,
	durableEvent *models.AgentDeliveryEvent,
	repository AgentDeliveryRepository,
	client *agentctl.Client,
	deferReceipt bool,
	attempt *deliveryStartupAttempt,
) (process, skipCallback bool, effect *models.AgentDeliveryEffect, deferred bool, err error) {
	if durableEvent == nil || durableEvent.StreamID == "" || durableEvent.Sequence == 0 {
		return true, false, nil, false, nil
	}
	stale, err := deliveryEventIsStale(ctx, execution, durableEvent, repository, attempt)
	if err != nil {
		return false, false, nil, false, err
	}
	if deferReceipt {
		if _, ok := repository.(canonicalAgentDeliveryBatchReceiver); ok {
			effect = deliveryEffectForDeliveryEvent(durableEvent)
			alreadyApplied, effectErr := sm.deliveryEffectAlreadyApplied(ctx, repository, effect)
			if effectErr != nil {
				return false, false, nil, false, effectErr
			}
			return true, stale || alreadyApplied, effect, true, nil
		}
	}
	process, err = sm.receiveDurableAgentDeliveryEvent(ctx, durableEvent, repository)
	if errors.Is(err, repoerrors.ErrAgentDeliveryReplayExpired) {
		projected, recoveryErr := reconcileExpiredAgentDeliveryReplay(
			ctx, execution, durableEvent, repository, client, client, sm.awaitDeliveryDurability,
		)
		if recoveryErr != nil {
			return false, false, nil, false, fmt.Errorf("reconcile expired durable replay: %w", recoveryErr)
		}
		execution.DeliveryReplayCursor = projected
		return false, false, nil, false, errDeliveryReplayReconciled
	}
	if err != nil {
		return false, false, nil, false, fmt.Errorf("admit durable agent event: %w", err)
	}
	effect = deliveryEffectForDeliveryEvent(durableEvent)
	alreadyApplied, err := sm.deliveryEffectAlreadyApplied(ctx, repository, effect)
	if err != nil {
		return false, false, nil, false, err
	}
	return process, stale || alreadyApplied, effect, false, nil
}

func (sm *StreamManager) projectCanonicalAgentEvent(
	ctx context.Context,
	execution *AgentExecution,
	event agentctl.AgentEvent,
	durableEvent *models.AgentDeliveryEvent,
	delivery AgentDeliveryRepository,
	client *agentctl.Client,
	scheduleAcknowledgement bool,
) (agentctl.AgentEvent, bool, bool, error) {
	if delivery == nil || !canonicalAgentDeliveryEvent(event) {
		return event, false, true, nil
	}
	if event.DeliveryStreamID == "" || event.DeliverySequence == 0 {
		return event, false, true, nil
	}
	if durableEvent == nil {
		durableEvent = durableAgentEvent(execution, event)
	}
	projector, ok := delivery.(canonicalAgentDeliveryProjector)
	if !ok {
		if event.Type == streams.EventTypeShellOutputUpdate {
			return event, false, false, ErrDeliveryRecoveryBlocked
		}
		sm.logger.Debug("canonical agent delivery projector is unavailable; using legacy projection",
			zap.String("instance_id", execution.ID),
			zap.String("event_type", event.Type))
		return event, false, true, nil
	}
	projectionResult, err := projector.ProjectCanonicalAgentDeliveryEvent(
		ctx, durableEvent, deliveryEffectForEvent(event),
	)
	if err != nil {
		sm.logger.Error("canonical agent event projection failed",
			zap.String("instance_id", execution.ID),
			zap.String("event_type", event.Type),
			zap.Error(err))
		return event, false, false, err
	}
	notify := true
	if event.Type == streams.EventTypeShellOutputUpdate || canonicalShellStartEvent(event) {
		event, err = committedShellProjectionEvent(ctx, delivery, event, durableEvent.SessionID)
		if err != nil {
			return event, false, false, err
		}
		if event.Type == streams.EventTypeToolCall {
			event.CanonicalMessageAppend = projectionResult.Appended
		}
	} else {
		event.CanonicalMessageBaseID = canonicalAgentMessageID(execution, event)
		event.CanonicalMessageID = projectionResult.CanonicalMessageID
		event.CanonicalMessageAppend = projectionResult.Appended
		notify = canonicalProjectionShouldNotify(event, projectionResult)
	}
	event.CanonicalProjection = true
	applyCanonicalCompactionResult(&event, projectionResult)
	// Projection is the canonical commit boundary. ACK transport is scheduled
	// independently so a lost ACK cannot hide output that is already durable.
	if scheduleAcknowledgement {
		sm.scheduleDurableDeliveryAck(execution, client, event)
	}
	return event, true, notify, nil
}

// commitPreparedAgentEvents commits one prepared batch. Batches prepared with
// deferred receipts admit and project through one writer transaction; all
// other batches keep the projection-only path.
func (sm *StreamManager) commitPreparedAgentEvents(
	ctx context.Context,
	execution *AgentExecution,
	prepared []preparedAgentEvent,
	delivery AgentDeliveryRepository,
	client *agentctl.Client,
	startupGeneration uint64,
	attempt *deliveryStartupAttempt,
) error {
	if len(prepared) == 0 {
		return nil
	}
	receiver, ok := delivery.(canonicalAgentDeliveryBatchReceiver)
	if !ok || !batchReceiptDeferred(prepared) {
		// A batch normally has one receipt mode. If a deferred item ever
		// reaches this path, replay it through admission instead of projecting
		// an unadmitted event.
		if batchHasDeferredReceipt(prepared) {
			return sm.replayPreparedAgentEventsWithReceipt(ctx, execution, prepared, delivery, client, startupGeneration, attempt)
		}
		return sm.projectCanonicalAgentEvents(ctx, execution, prepared, delivery, client, startupGeneration)
	}
	events, effects, err := canonicalDeliveryBatchInputs(prepared)
	if err != nil {
		return err
	}
	results, err := receiver.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, 0)
	if err != nil {
		if !isCombinedDeliveryAdmissionError(err) {
			return err
		}
		// The combined transaction rolled back, so the existing two-phase path
		// starts from the same SQL state. Expiry reconciliation and typed
		// recovery stay in prepareAgentEvent.
		return sm.replayPreparedAgentEventsWithReceipt(ctx, execution, prepared, delivery, client, startupGeneration, attempt)
	}
	if len(results) != len(prepared) {
		return fmt.Errorf("canonical delivery batch receiver returned %d results for %d events", len(results), len(prepared))
	}
	// Suppress duplicate notifications only on the combined path. The
	// single-event and projection-only paths keep their existing notification
	// behavior; their effect pre-check already handles replayed events.
	for index := range results {
		if results[index].AlreadyProjected {
			prepared[index].skipCallback = true
		}
	}
	sm.notifyCanonicalAgentEventBatch(execution, prepared, results, client, startupGeneration)
	return nil
}

func batchReceiptDeferred(prepared []preparedAgentEvent) bool {
	for _, item := range prepared {
		if !item.receiptDeferred {
			return false
		}
	}
	return true
}

func batchHasDeferredReceipt(prepared []preparedAgentEvent) bool {
	for _, item := range prepared {
		if item.receiptDeferred {
			return true
		}
	}
	return false
}

// replayPreparedAgentEventsWithReceipt reruns each deferred event through the
// individual admission and projection path. It is only used after the combined
// transaction rejected the batch without committing anything.
func (sm *StreamManager) replayPreparedAgentEventsWithReceipt(
	ctx context.Context,
	execution *AgentExecution,
	prepared []preparedAgentEvent,
	delivery AgentDeliveryRepository,
	client *agentctl.Client,
	startupGeneration uint64,
	attempt *deliveryStartupAttempt,
) error {
	for _, item := range prepared {
		retry, err := sm.prepareAgentEventWithReceipt(ctx, execution, client, delivery, item.event, false, attempt)
		if err != nil {
			return err
		}
		if retry.duplicate {
			continue
		}
		if err := sm.processPreparedAgentEvent(ctx, execution, client, delivery, retry, startupGeneration); err != nil {
			return err
		}
	}
	return nil
}

func isCombinedDeliveryAdmissionError(err error) bool {
	for _, sentinel := range []error{
		repoerrors.ErrAgentDeliveryReplayExpired,
		repoerrors.ErrAgentDeliveryEventConflict,
		repoerrors.ErrAgentDeliveryCheckpointMissing,
		repoerrors.ErrAgentDeliveryIdentityMismatch,
		repoerrors.ErrAgentDeliveryIdentityRetiring,
		repoerrors.ErrAgentDeliveryReceiptCorrupt,
		repoerrors.ErrAgentDeliveryHashVersion,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

// projectCanonicalAgentEvents applies one contiguous compatible batch before
// any lifecycle callback is notified. Repositories without the optional batch
// contract retain the single-event projection path.
func (sm *StreamManager) projectCanonicalAgentEvents(
	ctx context.Context,
	execution *AgentExecution,
	prepared []preparedAgentEvent,
	delivery AgentDeliveryRepository,
	client *agentctl.Client,
	startupGeneration uint64,
) error {
	if len(prepared) == 0 {
		return nil
	}
	if prepared[0].event.Type == streams.EventTypeShellOutputUpdate || prepared[0].event.Type == streams.EventTypeToolCall {
		return sm.projectCanonicalAgentEventsIndividually(ctx, execution, prepared, delivery, client, startupGeneration)
	}
	projector, ok := delivery.(canonicalAgentDeliveryBatchProjector)
	if !ok {
		return sm.projectCanonicalAgentEventsIndividually(
			ctx, execution, prepared, delivery, client, startupGeneration,
		)
	}

	events, effects, err := canonicalDeliveryBatchInputs(prepared)
	if err != nil {
		return err
	}
	projectionResults, err := projector.ProjectCanonicalAgentDeliveryEvents(ctx, events, effects)
	if err != nil {
		return err
	}
	if len(projectionResults) != len(prepared) {
		return fmt.Errorf("canonical delivery projector returned %d results for %d events", len(projectionResults), len(prepared))
	}
	sm.notifyCanonicalAgentEventBatch(execution, prepared, projectionResults, client, startupGeneration)
	return nil
}

func (sm *StreamManager) projectCanonicalAgentEventsIndividually(
	ctx context.Context,
	execution *AgentExecution,
	prepared []preparedAgentEvent,
	delivery AgentDeliveryRepository,
	client *agentctl.Client,
	startupGeneration uint64,
) error {
	for _, item := range prepared {
		event, _, notify, err := sm.projectCanonicalAgentEvent(
			ctx, execution, item.event, item.durableEvent, delivery, client, true,
		)
		if err != nil {
			return err
		}
		if notify && !item.skipCallback {
			sm.notifyAgentEvent(execution, event, startupGeneration)
		}
	}
	return nil
}

func canonicalDeliveryBatchInputs(
	prepared []preparedAgentEvent,
) ([]*models.AgentDeliveryEvent, []*models.AgentDeliveryEffect, error) {
	events := make([]*models.AgentDeliveryEvent, len(prepared))
	effects := make([]*models.AgentDeliveryEffect, len(prepared))
	for i, item := range prepared {
		if item.durableEvent == nil || !canonicalAgentDeliveryEvent(item.event) {
			return nil, nil, fmt.Errorf("canonical delivery batch contains a non-canonical event")
		}
		events[i] = item.durableEvent
		effects[i] = item.deliveryEffect
	}
	return events, effects, nil
}

func (sm *StreamManager) notifyCanonicalAgentEventBatch(
	execution *AgentExecution,
	prepared []preparedAgentEvent,
	projectionResults []models.CanonicalAgentDeliveryProjectionResult,
	client *agentctl.Client,
	startupGeneration uint64,
) {
	var notification agentctl.AgentEvent
	var notify bool
	var content strings.Builder
	var compactionCount *int64
	var compactionReporting *streams.ContextCompactionReporting
	for i, item := range prepared {
		event := item.event
		event.CanonicalMessageBaseID = canonicalAgentMessageID(execution, event)
		event.CanonicalMessageID = projectionResults[i].CanonicalMessageID
		event.CanonicalProjection = true
		event.CanonicalMessageAppend = projectionResults[i].Appended
		applyCanonicalCompactionResult(&event, projectionResults[i])
		if count := projectionResults[i].ContextCompactionCount; count != nil {
			value := *count
			compactionCount = &value
		}
		if reporting := projectionResults[i].ContextCompactionReporting; reporting != nil {
			value := *reporting
			compactionReporting = &value
		}
		sm.scheduleDurableDeliveryAck(execution, client, event)
		if item.skipCallback || !canonicalProjectionShouldNotify(event, projectionResults[i]) {
			continue
		}
		if !notify {
			notification = event
			notify = true
		}
		if event.Type == streams.EventTypeReasoning {
			content.WriteString(event.ReasoningText)
		} else {
			content.WriteString(event.Text)
		}
	}
	if notify {
		applyCanonicalCompactionMetadata(&notification, compactionCount, compactionReporting)
		if notification.Type == streams.EventTypeReasoning {
			notification.ReasoningText = content.String()
		} else {
			notification.Text = content.String()
		}
		sm.notifyAgentEvent(execution, notification, startupGeneration)
	}
}

func canonicalProjectionShouldNotify(
	event agentctl.AgentEvent,
	result models.CanonicalAgentDeliveryProjectionResult,
) bool {
	if event.Type != streams.EventTypeMessageChunk && event.Type != streams.EventTypeReasoning {
		return true
	}
	return result.CanonicalMessageID != ""
}

// durableAgentEventIdentity distinguishes a genuine legacy event from a
// malformed event from an execution that already negotiated retained
// delivery. Legacy callbacks must remain usable when a repository is present,
// while a v1 stream must fail closed if its cursor identity is absent.
func durableAgentEventIdentity(execution *AgentExecution, event agentctl.AgentEvent) (bool, error) {
	if event.DeliveryStreamID != "" && event.DeliverySequence > 0 {
		if !durableAgentEventIdentityMatches(execution, event) {
			return false, ErrDurableAdoptionIdentityMismatch
		}
		return true, nil
	}
	if durableAgentEventIdentityPartial(event) || durableDeliveryExpected(execution) {
		return false, fmt.Errorf("%w: durable agent event identity is missing", ErrUncertainPromptDelivery)
	}
	return false, nil
}

func durableAgentEventIdentityMatches(execution *AgentExecution, event agentctl.AgentEvent) bool {
	return execution != nil &&
		execution.DeliveryMode == DurableDeliveryV1 &&
		execution.DeliveryStreamID != "" &&
		execution.DeliveryStreamID == event.DeliveryStreamID &&
		execution.DeliveryIncarnationID != "" &&
		execution.DeliveryIncarnationID == event.DeliveryIncarnationID &&
		execution.DeliveryHarnessGeneration != 0 &&
		execution.DeliveryHarnessGeneration == event.DeliveryHarnessGeneration
}

func durableAgentEventIdentityPartial(event agentctl.AgentEvent) bool {
	return event.DeliveryStreamID != "" ||
		event.DeliveryIncarnationID != "" ||
		event.DeliveryHarnessGeneration != 0 ||
		event.DeliverySequence != 0
}

func durableDeliveryExpected(execution *AgentExecution) bool {
	return execution != nil && execution.DeliveryMode == DurableDeliveryV1
}
