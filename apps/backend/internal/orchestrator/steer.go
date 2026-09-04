package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// ErrSteerNotEligible means the session cannot be steered right now: the
// experiment is off, the connected agent did not advertise prompt queueing, or
// the session is not in a generating RUNNING state. The caller falls back to its
// ordinary prompt path (the session may since have become promptable).
var ErrSteerNotEligible = errors.New("session is not eligible for mid-turn steering")

// steerQueuedStopReason marks a PromptResult whose steer was enqueued behind
// pending work rather than dispatched immediately. Callers treat it as success.
const steerQueuedStopReason = "steer_queued"

// SteerEligible reports whether a prompt to sessionID right now would be
// delivered as a mid-turn steer rather than queued. It is a pure read used by
// the composer contract and by the message handler before it decides to steer.
//
// Eligibility requires all of: the runtime experiment enabled, the connected
// agent's negotiated prompt-queueing advertisement, and a RUNNING session whose
// foreground is generating (a background-idle session already accepts prompts
// through the handoff path and does not need steering).
func (s *Service) SteerEligible(sessionID string, state models.TaskSessionState) bool {
	if !s.config.ClaudeMidTurnSteering {
		return false
	}
	if state != models.TaskSessionStateRunning {
		return false
	}
	if !s.sessionAdvertisesPromptQueueing(sessionID) {
		return false
	}
	// A background-idle session is already promptable via the handoff path;
	// steering is specifically for a foreground that is still generating.
	//
	// Use the raw foregroundActivityValue, not the public ForegroundActivity:
	// the latter collapses to Generating whenever ClaudeBackgroundPromptHandoff
	// is off, and that flag is independent of ClaudeMidTurnSteering. Reading it
	// here would make a background-idle session steer-eligible with only
	// mid-turn steering enabled, which is exactly the case steering must exclude.
	return s.foregroundActivityValue(sessionID) == v1.ForegroundActivityGenerating
}

// steerEligibleForSession resolves the session state and reports steer
// eligibility for the WS activity payload. Fails closed to false on any lookup
// error, matching the serialization-boundary derivation used by the DTOs.
func (s *Service) steerEligibleForSession(ctx context.Context, sessionID string) bool {
	if s.repo == nil {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return false
	}
	return s.SteerEligible(sessionID, session.State)
}

// SteerTask delivers a prompt into a still-generating turn. It enforces the two
// ordering invariants from the spec — steer only with an empty queue, and at
// most one in-flight steer per session — then dispatches without taking the
// foreground claim, which the predecessor turn still owns.
//
// When either invariant would be violated, the prompt is enqueued (joining the
// tail so submission order is preserved) and drained on the next turn boundary
// rather than dispatched now; the caller sees success. ErrSteerNotEligible is
// returned only when the session is no longer steer-eligible at all (e.g. its
// turn ended between the caller's check and here), so the caller can fall back
// to an ordinary prompt. If attachment materialization fails after admission,
// the failed owner releases its slot and retries a queued drain so a completion
// that raced with the failure cannot strand the successor. Any other error is a
// genuine dispatch failure.
func (s *Service) SteerTask(
	ctx context.Context,
	taskID, sessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
) (*PromptResult, error) {
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return nil, err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil, ErrSteerNotEligible
	}
	if !s.SteerEligible(sessionID, session.State) {
		return nil, ErrSteerNotEligible
	}

	var (
		admittedForDispatch bool
		queueNonEmpty       bool
		steerOutstanding    bool
		admittedResult      *PromptResult
	)
	if err := s.messageQueue.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		// Order rule: never jump ahead of an already-queued message, and never run
		// two steers at once. Either way, enqueue behind whatever is pending so the
		// message is delivered in order on the next turn boundary.
		if status := s.messageQueue.GetStatus(admittedCtx, sessionID); status != nil && status.Count > 0 {
			queueNonEmpty = true
		}
		_, steerOutstanding = s.steerInFlight.LoadOrStore(sessionID, struct{}{})
		if queueNonEmpty || steerOutstanding {
			// LoadOrStore may have installed our marker when the queue was the reason
			// we decline; only delete it if no genuine prior steer owns it.
			if !steerOutstanding {
				s.steerInFlight.Delete(sessionID)
			}
			_, qErr := s.messageQueue.QueueMessageWithMetadata(
				admittedCtx, sessionID, taskID, prompt, model, messagequeue.QueuedByUser,
				planMode, toQueuedAttachments(attachments), nil,
			)
			if qErr != nil {
				return fmt.Errorf("queue steer behind pending work: %w", qErr)
			}
			// SteerTask writes through the queue service directly, so it must
			// publish the same authoritative snapshot as the WS queue handlers.
			// Without this event, clients keep rendering the queue state from
			// before the steer was accepted.
			s.publishQueueStatusEvent(admittedCtx, sessionID)
			admittedResult = &PromptResult{StopReason: steerQueuedStopReason}
			return nil
		}
		admittedForDispatch = true
		return nil
	}); err != nil {
		return nil, err
	}
	if !admittedForDispatch {
		s.logger.Info("steer enqueued behind pending work",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Bool("queue_non_empty", queueNonEmpty),
			zap.Bool("steer_outstanding", steerOutstanding))
		return admittedResult, nil
	}
	// We now own the single in-flight slot. The queue admission lock is released
	// before the blocking dispatch; clear the slot when the dispatch completes.
	promptCtx := context.WithoutCancel(ctx)
	var dispatchErr error
	defer func() {
		s.steerInFlight.Delete(sessionID)
		if errors.Is(dispatchErr, executor.ErrSteerAttachmentMaterialization) {
			// A predecessor completion may have skipped its drain while this
			// marker was set. Retry after releasing the marker so the queued
			// successor can make progress if the session is promptable now.
			s.drainQueuedMessageForPromptableSession(promptCtx, sessionID)
		}
	}()

	effectivePrompt := s.effectivePromptForSession(sessionID, prompt, planMode, session)
	s.logger.Info("dispatching mid-turn steer",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	// context.WithoutCancel: a WS request timeout must not abort the steer, same
	// as the ordinary prompt path.
	// dispatchOnly=true: a steer is dispatch-and-continue. The predecessor turn
	// owns foreground completion, so this call must not wait for a turn to end,
	// and must not take the foreground claim.
	result, dispatchErr := s.executor.SteerWithDispatchCallback(
		promptCtx, taskID, sessionID, effectivePrompt, attachments, true, func() {}, session,
	)
	if dispatchErr != nil {
		if errors.Is(dispatchErr, executor.ErrSteerNotDispatched) {
			// The active generation ended before the steer reached agentctl.
			// Let the handler re-enter PromptTask, which owns ordinary queue and
			// foreground admission for the replacement turn.
			return nil, ErrSteerNotEligible
		}
		s.logger.Warn("mid-turn steer dispatch failed",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(dispatchErr))
		return nil, dispatchErr
	}
	return &PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// isSteerInFlight reports whether sessionID has a steer admitted but not yet
// dispatched (see steerInFlight's field doc comment). The non-cancelling
// queue-drain paths — drainQueuedMessageForPromptableSessionLocked and
// takeIfPromptableLocked — must back off while this is true, the same way
// they already back off on isQueuedDispatchInFlight: SteerTask releases the
// message-queue admission lock before its blocking dispatch, so between
// admission and dispatch a message can be queued and the turn can complete,
// and without this check an ordinary drain would dispatch that later message
// ahead of the steer that was admitted first — reopening the ordering gap
// WithSessionAdmission's shared lock was meant to close.
func (s *Service) isSteerInFlight(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	_, ok := s.steerInFlight.Load(sessionID)
	return ok
}
