package lifecycle

import (
	"context"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
)

// turnOutcomeApplier is the optional capability satisfied by
// *StandaloneExecutor: only standalone-backed instances have agentctl turn-
// outcome retention (AC-EXECUTORS-SURVIVAL-004) -- this capability covers the
// WORKTREE and LOCAL executor types only, both standalone-runtime. A backend
// that doesn't implement it (Docker, Sprites, SSH, Kubernetes) is out of
// scope, and recovery treats that exactly like "nothing retained."
type turnOutcomeApplier interface {
	fetchTurnOutcomeWithRetry(ctx context.Context, instanceID string) (*agentctl.TurnOutcome, error)
	ackTurnOutcome(ctx context.Context, instanceID string, turnID int64) error
}

// recoveredTurnOutcomeResult is the outcome of retrieveRecoveredTurnOutcome:
// exactly one of the three cases AC-EXECUTORS-SURVIVAL-004.5 distinguishes.
type recoveredTurnOutcomeResult int

const (
	// recoveredTurnOutcomeNone: the read succeeded and nothing was retained,
	// or the backend does not support turn-outcome retention at all. Publish
	// the session as running.
	recoveredTurnOutcomeNone recoveredTurnOutcomeResult = iota
	// recoveredTurnOutcomeApplied: an outcome was retrieved and must be applied.
	recoveredTurnOutcomeApplied
	// recoveredTurnOutcomeReadFailed: the read failed and its retries are
	// exhausted. AC-EXECUTORS-SURVIVAL-004.5 forbids publishing running here;
	// the instance goes to the not-re-tracked path instead.
	recoveredTurnOutcomeReadFailed
)

// retrieveRecoveredTurnOutcome implements AC-EXECUTORS-SURVIVAL-004.2's read
// step for one recovered instance, bounded by the same per-read timeout and
// retry count as AC-EXECUTORS-SURVIVAL-002.13 (AC-EXECUTORS-SURVIVAL-004.5).
func (m *Manager) retrieveRecoveredTurnOutcome(ctx context.Context, ri *ExecutorInstance) (*agentctl.TurnOutcome, recoveredTurnOutcomeResult) {
	backend, err := m.executorRegistry.GetBackend(ri.RuntimeName)
	if err != nil {
		return nil, recoveredTurnOutcomeNone
	}
	applier, ok := backend.(turnOutcomeApplier)
	if !ok {
		return nil, recoveredTurnOutcomeNone
	}
	outcome, err := applier.fetchTurnOutcomeWithRetry(ctx, ri.StandaloneInstanceID)
	if err != nil {
		m.logger.Warn("failed to retrieve recovered instance's turn outcome after exhausting retries",
			zap.String("instance_id", ri.InstanceID),
			zap.String("session_id", ri.SessionID),
			zap.Error(err))
		return nil, recoveredTurnOutcomeReadFailed
	}
	if outcome == nil {
		return nil, recoveredTurnOutcomeNone
	}
	return outcome, recoveredTurnOutcomeApplied
}

// applyRecoveredTurnOutcome implements AC-EXECUTORS-SURVIVAL-004.2's apply
// step. It seeds the execution's prompt-completion generation guard from the
// retained event's own PromptGeneration -- recreated from zero by recovery,
// per design 03's "Turn outcome across the detached gap" -- so the existing
// completion-claim path in handleAgentEvent (via claimPromptCompletion)
// processes this event exactly as it would have had a backend been attached
// when the turn ended, publishing whatever state that produces. It records
// the applied control-server turn identifier before processing so a live
// redelivery of the same event (agentctl's parked send finally unblocked by
// this stream reconnecting) is recognized and dropped
// (AC-EXECUTORS-SURVIVAL-004.4, see AgentExecution.isRecoveryDuplicateEvent).
//
// The acknowledgement is sent only after handleAgentEvent returns
// (AC-EXECUTORS-SURVIVAL-004.6): on the default in-memory event bus, Publish
// delivers to subscribers synchronously before returning, so the
// orchestrator's session-state and task-state writes are both complete by
// then; on a NATS-backed bus this ordering is best-effort, the same
// documented precedent internal/orchestrator/event_handlers_streaming.go's
// markReadyTurn comment relies on for durable-TurnID-based ordering. A failed
// or best-effort-late acknowledgement is not retried here: the outcome stays
// retained and a later backend re-applies it idempotently
// (AC-EXECUTORS-SURVIVAL-004.6's "applying to a turn that already reached
// that outcome... changes nothing").
func (m *Manager) applyRecoveredTurnOutcome(
	ctx context.Context, execution *AgentExecution, ri *ExecutorInstance, outcome *agentctl.TurnOutcome,
) {
	_ = m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		current.promptGeneration = outcome.Event.PromptGeneration
	})
	execution.markRecoveryTurnOutcomeApplied(outcome.TurnID)

	m.handleAgentEvent(execution, outcome.Event)

	backend, err := m.executorRegistry.GetBackend(ri.RuntimeName)
	if err != nil {
		return
	}
	applier, ok := backend.(turnOutcomeApplier)
	if !ok {
		return
	}
	if err := applier.ackTurnOutcome(ctx, ri.StandaloneInstanceID, outcome.TurnID); err != nil {
		m.logger.Warn("failed to acknowledge applied turn outcome; a later backend will re-apply it idempotently",
			zap.String("instance_id", ri.InstanceID),
			zap.Int64("turn_id", outcome.TurnID),
			zap.Error(err))
	}
}

// publishRecoveredExecutionRunning implements AC-EXECUTORS-SURVIVAL-004.5's
// "nothing retained" case: publish the session as running exactly as an
// ordinary launch's first activity would.
func (m *Manager) publishRecoveredExecutionRunning(ctx context.Context, execution *AgentExecution) {
	// Nothing was retained, so any turn still in flight across the restart
	// carries a PromptGeneration this freshly reconstructed execution object
	// has never seen (its own counter was recreated from zero). Let the first
	// live completion adopt it instead of being rejected as superseded --
	// see recoveredPromptGenerationPending.
	execution.recoveredPromptGenerationPending.Store(true)
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentRunning, execution)
}
