package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
	return s.ForegroundActivity(sessionID) == v1.ForegroundActivityGenerating
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

// steerAdmitLock returns the per-session mutex that serializes a steer's
// admission decision. The queue-empty check, the single-in-flight claim, and the
// enqueue-on-decline must happen as one critical section: without it a message
// could enter the queue between the check and the claim and the steer would jump
// it, and two concurrent steers could both pass the check.
func (s *Service) steerAdmitLock(sessionID string) *sync.Mutex {
	actual, _ := s.steerAdmitLocks.LoadOrStore(sessionID, &sync.Mutex{})
	return actual.(*sync.Mutex)
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
// to an ordinary prompt. Any other error is a genuine dispatch failure.
func (s *Service) SteerTask(
	ctx context.Context,
	taskID, sessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
) (*PromptResult, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil, ErrSteerNotEligible
	}
	if !s.SteerEligible(sessionID, session.State) {
		return nil, ErrSteerNotEligible
	}

	// Serialize the admission decision for this session so the ordering and
	// single-in-flight invariants cannot race. The lock guards only the
	// check-claim-or-enqueue critical section below; it is released before the
	// (blocking) dispatch so a later send for the same session can make its own
	// admission decision promptly.
	admit := s.steerAdmitLock(sessionID)
	admit.Lock()

	// Order rule: never jump ahead of an already-queued message, and never run
	// two steers at once. Either way, enqueue behind whatever is pending so the
	// message is delivered in order on the next turn boundary.
	queueNonEmpty := false
	if status := s.messageQueue.GetStatus(ctx, sessionID); status != nil && status.Count > 0 {
		queueNonEmpty = true
	}
	_, steerOutstanding := s.steerInFlight.LoadOrStore(sessionID, struct{}{})
	if queueNonEmpty || steerOutstanding {
		// LoadOrStore may have installed our marker when the queue was the reason
		// we decline; only delete it if no genuine prior steer owns it.
		if !steerOutstanding {
			s.steerInFlight.Delete(sessionID)
		}
		admit.Unlock()
		if _, qErr := s.messageQueue.QueueMessageWithMetadata(
			ctx, sessionID, taskID, prompt, model, messagequeue.QueuedByUser,
			planMode, toQueuedAttachments(attachments), nil,
		); qErr != nil {
			return nil, fmt.Errorf("queue steer behind pending work: %w", qErr)
		}
		s.logger.Info("steer enqueued behind pending work",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Bool("queue_non_empty", queueNonEmpty),
			zap.Bool("steer_outstanding", steerOutstanding))
		return &PromptResult{StopReason: steerQueuedStopReason}, nil
	}
	// We now own the single in-flight slot. Release the admission lock before the
	// blocking dispatch; clear the slot when the dispatch completes.
	admit.Unlock()
	defer s.steerInFlight.Delete(sessionID)

	effectivePrompt := s.effectivePromptForSession(sessionID, prompt, planMode, session)
	s.logger.Info("dispatching mid-turn steer",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))

	// context.WithoutCancel: a WS request timeout must not abort the steer, same
	// as the ordinary prompt path.
	promptCtx := context.WithoutCancel(ctx)
	// dispatchOnly=true: a steer is dispatch-and-continue. The predecessor turn
	// owns foreground completion, so this call must not wait for a turn to end,
	// and must not take the foreground claim.
	result, err := s.executor.SteerWithDispatchCallback(
		promptCtx, taskID, sessionID, effectivePrompt, attachments, true, func() {}, session,
	)
	if err != nil {
		s.logger.Warn("mid-turn steer dispatch failed",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, err
	}
	return &PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}
