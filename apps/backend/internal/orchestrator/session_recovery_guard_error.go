package orchestrator

const detailsKeyKind = "kind"

// SessionRecoveryGuardError identifies a session launch refused by the
// startup recovery guard. It wraps the original sentinel error so callers
// can still use errors.Is/errors.As.
type SessionRecoveryGuardError struct {
	Cause     error
	SessionID string
	Retryable bool
}

func (e *SessionRecoveryGuardError) Error() string {
	if e == nil {
		return "session launch is blocked by the recovery guard"
	}
	if e.Retryable {
		return "the session is still being reconciled after a backend restart; try again shortly"
	}
	return "the session's agent could not be stopped after a backend restart and requires another restart before it can be launched"
}

func (e *SessionRecoveryGuardError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Details returns the stable WebSocket recovery contract for this error.
func (e *SessionRecoveryGuardError) Details() map[string]interface{} {
	if e == nil {
		return nil
	}
	kind := "session_recovery_unstoppable"
	if e.Retryable {
		kind = "session_recovery_in_progress"
	}
	return map[string]interface{}{
		detailsKeyKind:   kind,
		"retryable":      e.Retryable,
		metaKeySessionID: e.SessionID,
	}
}
