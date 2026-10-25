package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// SessionRecoveryIdentity identifies the recovery operation that produced a
// prompt failure. Attempt and execution IDs are process/runtime identities;
// ErrorStamp is the durable identity of the rendered recovery record.
type SessionRecoveryIdentity struct {
	AttemptID   string
	ExecutionID string
	ErrorStamp  string
}

// SessionRecoveryFailure keeps the original error while correlating it to the
// recovery record that may already explain the failure to the user.
type SessionRecoveryFailure struct {
	Err      error
	Identity SessionRecoveryIdentity
}

func (e *SessionRecoveryFailure) Error() string {
	if e == nil || e.Err == nil {
		return "session recovery failed"
	}
	return e.Err.Error()
}

func (e *SessionRecoveryFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RecoveryFailureIdentity extracts the concrete identity carried by err.
func RecoveryFailureIdentity(err error) (SessionRecoveryIdentity, bool) {
	var failure *SessionRecoveryFailure
	if !errors.As(err, &failure) || failure == nil {
		return SessionRecoveryIdentity{}, false
	}
	return failure.Identity, true
}

func (s *Service) withSessionRecoveryFailureIdentity(
	ctx context.Context,
	taskID, sessionID string,
	attempt *resumeAttempt,
	executionID string,
	err error,
) error {
	if err == nil || errors.Is(err, ErrResumeAttemptCancelled) {
		return err
	}
	if isSessionRecoveryFailure(err) {
		return err
	}
	identity := sessionRecoveryIdentity(attempt, executionID)
	identity.ErrorStamp = s.matchingSessionRecoveryErrorStamp(
		ctx, taskID, sessionID, identity,
	)
	return &SessionRecoveryFailure{Err: err, Identity: identity}
}

func isSessionRecoveryFailure(err error) bool {
	var existing *SessionRecoveryFailure
	return errors.As(err, &existing)
}

func sessionRecoveryIdentity(attempt *resumeAttempt, executionID string) SessionRecoveryIdentity {
	identity := SessionRecoveryIdentity{ExecutionID: executionID}
	if attempt == nil {
		return identity
	}
	identity.AttemptID = attempt.identity()
	if identity.ExecutionID == "" {
		identity.ExecutionID = attempt.execution()
	}
	return identity
}

func (s *Service) matchingSessionRecoveryErrorStamp(
	ctx context.Context,
	taskID, sessionID string,
	identity SessionRecoveryIdentity,
) string {
	lookupCtx := context.Background()
	if ctx != nil {
		lookupCtx = context.WithoutCancel(ctx)
	}
	if s == nil || s.repo == nil || taskID == "" || sessionID == "" {
		return ""
	}
	session, err := s.repo.GetTaskSession(lookupCtx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return ""
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastError.IsDismissed() || !recoveryIdentityMatchesLastError(identity, lastError) {
		return ""
	}
	return lastError.Stamp()
}

// HasActiveSessionRecoveryForFailure reports ownership only for the concrete
// recovery failure being returned. A historical or unrelated error cannot
// suppress the new failure message.
func (s *Service) HasActiveSessionRecoveryForFailure(
	ctx context.Context,
	taskID, sessionID string,
	failure error,
) bool {
	if s == nil || s.repo == nil || taskID == "" || sessionID == "" {
		return false
	}
	identity, ok := RecoveryFailureIdentity(failure)
	if !ok || (identity.AttemptID == "" && identity.ExecutionID == "" && identity.ErrorStamp == "") {
		return false
	}
	lookupCtx := context.Background()
	if ctx != nil {
		lookupCtx = context.WithoutCancel(ctx)
	}
	session, err := s.repo.GetTaskSession(lookupCtx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return false
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	return ok && !lastError.IsDismissed() && recoveryIdentityMatchesLastError(identity, lastError)
}

func recoveryIdentityMatchesLastError(identity SessionRecoveryIdentity, lastError models.LastAgentError) bool {
	if identity.AttemptID != "" && lastError.AttemptID != identity.AttemptID {
		return false
	}
	if identity.ExecutionID != "" {
		storedExecutionID := lastError.ExecutionID
		if storedExecutionID == "" {
			storedExecutionID = lastError.AgentExecutionID
		}
		if storedExecutionID != identity.ExecutionID {
			return false
		}
	}
	if identity.ErrorStamp != "" && !lastError.MatchesStamp(identity.ErrorStamp) {
		return false
	}
	return true
}
