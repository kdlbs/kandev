package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// ErrSteerNotEligible means the session cannot be steered right now: the
// experiment is off, the connected agent did not advertise prompt queueing, or
// the session is not in a generating RUNNING state. The caller falls back to its
// ordinary queue/block behavior.
var ErrSteerNotEligible = errors.New("session is not eligible for mid-turn steering")

// ErrSteerWouldReorder means a message is already queued ahead of this one, so
// steering would jump the queue. The caller must queue instead, preserving order.
var ErrSteerWouldReorder = errors.New("cannot steer while messages are queued")

// ErrSteerInFlight means an unacknowledged steer is already outstanding for this
// session. The caller must queue instead.
var ErrSteerInFlight = errors.New("a steer is already in flight for this session")

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

// SteerTask delivers a prompt into a still-generating turn. It enforces the two
// ordering invariants from the spec — steer only with an empty queue, and at
// most one in-flight steer per session — then dispatches without taking the
// foreground claim, which the predecessor turn still owns.
//
// It returns a typed sentinel (ErrSteerNotEligible / ErrSteerWouldReorder /
// ErrSteerInFlight) when the prompt must be queued instead; the caller maps
// those to its existing queue path. Any other error is a genuine dispatch
// failure.
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

	// Order rule: never jump ahead of an already-queued message.
	if status := s.messageQueue.GetStatus(ctx, sessionID); status != nil && status.Count > 0 {
		return nil, ErrSteerWouldReorder
	}

	// At most one unacknowledged steer per session. LoadOrStore makes the
	// check-and-claim atomic against a concurrent second steer.
	if _, loaded := s.steerInFlight.LoadOrStore(sessionID, struct{}{}); loaded {
		return nil, ErrSteerInFlight
	}
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
