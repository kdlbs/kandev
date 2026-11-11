package orchestrator

import (
	"context"
	"errors"
	"fmt"
)

// Scoped session authorization: prompting and controlling an agent are
// writes, distinct from merely reaching the session.
//
// Split from service.go, which is already past the 800-line limit.

// ErrScopeCheckerUnavailable reports that a scoped authorization check was
// required but not wired. It denies rather than degrading to a broader check,
// so a wiring mistake is a visible failure instead of a silent widening.
var ErrScopeCheckerUnavailable = errors.New("authorization is unavailable for this operation")

// authorizeSessionControl guards stopping or cancelling a running agent.
// Interrupting someone else's turn is a write, not a read, so it needs
// session.control rather than mere reach. Falls back to the reach check when
// no scoped checker is wired, so an unwired build is never more permissive.
func (s *Service) authorizeSessionControl(ctx context.Context, sessionID string) error {
	return s.authorizeSessionScoped(ctx, sessionID, s.sessionControlCheck)
}

// authorizeSessionPrompt guards starting, resuming, steering or dispatching an
// agent turn. Prompting is a write: a viewer who may read a transcript must
// not be able to put an agent to work.
func (s *Service) authorizeSessionPrompt(ctx context.Context, sessionID string) error {
	return s.authorizeSessionScoped(ctx, sessionID, s.sessionPromptCheck)
}

// authorizeSessionScoped applies a scoped session check.
//
// The fallback is limited to instances with NO session authorization wired at
// all, which is the auth-disabled single-user case. When access checking is
// wired but the scoped checker is not, that is a misconfiguration and the
// operation is denied: falling back to the reach check there would let a
// viewer stop or steer somebody else's running agent.
func (s *Service) authorizeSessionScoped(
	ctx context.Context, sessionID string, check func(context.Context, string) error,
) error {
	if sessionID == "" {
		return nil
	}
	if check != nil {
		return check(ctx, sessionID)
	}
	if s.sessionAccessCheck == nil {
		return nil
	}
	return ErrScopeCheckerUnavailable
}

// SetSessionPromptChecker installs the session.prompt boundary.
func (s *Service) SetSessionPromptChecker(check func(ctx context.Context, sessionID string) error) {
	s.sessionPromptCheck = check
}

// authorizeTaskPrompt is the task-keyed sibling of authorizeSessionPrompt, for
// entry points that name a task rather than a session. Same fallback rule.
func (s *Service) authorizeTaskPrompt(ctx context.Context, taskID string) error {
	if taskID == "" {
		return nil
	}
	if s.taskPromptCheck != nil {
		return s.taskPromptCheck(ctx, taskID)
	}
	if s.taskAccessCheck == nil {
		return nil
	}
	return ErrScopeCheckerUnavailable
}

// SetTaskPromptChecker installs the task-keyed session.prompt boundary.
func (s *Service) SetTaskPromptChecker(check func(ctx context.Context, taskID string) error) {
	s.taskPromptCheck = check
}

// authorizeTaskSessionPromptPair applies the prompt boundary to both IDs and
// keeps their pair consistent. Peer-message start is a prompt operation, so it
// must not require the separate read grant.
func (s *Service) authorizeTaskSessionPromptPair(ctx context.Context, taskID, sessionID string) error {
	if err := s.authorizeSessionPrompt(ctx, sessionID); err != nil {
		return err
	}
	if err := s.authorizeTaskPrompt(ctx, taskID); err != nil {
		return err
	}
	if taskID == "" || sessionID == "" || s.repo == nil {
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return nil //nolint:nilerr // both IDs authorized; consistency is best-effort
	}
	if session.TaskID != taskID {
		return fmt.Errorf("%w: session %s does not belong to task %s", ErrTaskSessionPairMismatch, sessionID, taskID)
	}
	return nil
}
