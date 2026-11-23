package orchestrator

import (
	"context"
	"errors"
)

// ErrCancelTargetChanged reports that the execution or turn a caller observed
// no longer owns the session, so the cancellation was not applied.
var ErrCancelTargetChanged = errors.New("cancel target changed")

// CancelTarget identifies the work a caller observed and wants interrupted.
// TurnID is empty when the session had no active turn record.
type CancelTarget struct {
	ExecutionID string
	TurnID      string
}

type cancelTargetContextKey struct{}

// ObserveCancelTarget reads the session's live execution and active turn.
func (s *Service) ObserveCancelTarget(ctx context.Context, sessionID string) (CancelTarget, error) {
	if s.agentManager == nil {
		return CancelTarget{}, errors.New("observe cancel target: agent manager is not configured")
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if err != nil {
		return CancelTarget{}, err
	}
	turnID, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil {
		return CancelTarget{}, err
	}
	return CancelTarget{ExecutionID: executionID, TurnID: turnID}, nil
}

// CancelAgentIfTarget is CancelAgent bound to an observed target. The target is
// compared again under the session's in-flight cancel guard, so a turn that
// finished and was replaced after the observation is never interrupted.
func (s *Service) CancelAgentIfTarget(ctx context.Context, sessionID string, target CancelTarget) error {
	return s.CancelAgent(context.WithValue(ctx, cancelTargetContextKey{}, target), sessionID)
}

// verifyCancelTarget enforces the target carried by CancelAgentIfTarget. It is
// a no-op for every other cancellation.
func (s *Service) verifyCancelTarget(ctx context.Context, sessionID string) error {
	target, ok := ctx.Value(cancelTargetContextKey{}).(CancelTarget)
	if !ok {
		return nil
	}
	current, err := s.ObserveCancelTarget(ctx, sessionID)
	if err != nil {
		return err
	}
	if current != target {
		return ErrCancelTargetChanged
	}
	return nil
}
