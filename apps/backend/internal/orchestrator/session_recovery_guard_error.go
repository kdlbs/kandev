package orchestrator

import "errors"

const detailsKeyKind = "kind"

// SessionRecoveryGuardError identifies a session launch refused by the
// startup recovery guard. It wraps the original sentinel error so callers
// can still use errors.Is/errors.As.
type SessionRecoveryGuardError struct {
	Cause     error
	SessionID string
	Retryable bool
	// Code is the stable recovery-guard code for a Created-session Nomad
	// recovery ("nomad_recovery_starting" or "nomad_recovery_operator_action").
	// Empty for every other guard.
	Code string
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

func isRetryableSessionRecoveryGuardError(err error) bool {
	var guardErr *SessionRecoveryGuardError
	return errors.As(err, &guardErr) && guardErr.Retryable
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
	details := map[string]interface{}{
		detailsKeyKind:   kind,
		"retryable":      e.Retryable,
		metaKeySessionID: e.SessionID,
	}
	if e.Code != "" {
		details["code"] = e.Code
	}
	return details
}
